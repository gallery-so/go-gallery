package publicapi

import (
	heappkg "container/heap"
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/recommend"
	"github.com/mikeydub/go-gallery/service/recommend/userpref"
	"github.com/mikeydub/go-gallery/service/redis"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
	"github.com/mikeydub/go-gallery/validate"
)

const trendingFeedCacheKey = "trending:feedEvents:all"

var feedOpts = struct {
	FreshnessFactor     float64 // extra weight added to a new post
	FirstPostFactor     float64 // extra weight added to a first post
	LookbackWindow      float64 // how far back to look for posts
	FreshnessWindow     float64 // how long a post is considered new
	PostHalfLife        float64 // controls the decay rate of posts
	GalleryPostHalfLife float64 // controls the decay rate of Gallery posts
	GalleryDecayPeriod  float64 // time it takes for a Gallery post to reach GalleryPostHalfLife from PostHalfLife
	FetchSize           int     // number of posts to include on the feed
	StreakThreshold     int     // number of posts before a streak is counted
	StreakFactor        float64 // factor that controls the impact of each extra post
	PostSpan            float64 // the max time between posts allowed for a post to be part of the same group
}{
	FreshnessFactor:     1.5,
	FirstPostFactor:     2.0,
	LookbackWindow:      time.Duration(4 * 24 * time.Hour).Minutes(),
	FreshnessWindow:     time.Duration(3 * time.Hour).Minutes(),
	PostHalfLife:        time.Duration(6 * time.Hour).Minutes(),
	GalleryPostHalfLife: time.Duration(10 * time.Hour).Minutes(),
	GalleryDecayPeriod:  time.Duration(4 * 24 * time.Hour).Minutes(),
	FetchSize:           128,
	StreakThreshold:     2,
	StreakFactor:        1.8,
	PostSpan:            time.Duration(time.Minute).Seconds(),
}

type FeedAPI struct {
	repos              *postgres.Repositories
	queries            *db.Queries
	loaders            *dataloader.Loaders
	validator          *validator.Validate
	ethClient          *ethclient.Client
	cache              *redis.Cache
	taskClient         *task.Client
	multichainProvider *multichain.Provider
}

func (api FeedAPI) BanUser(ctx context.Context, userId persist.DBID, reason persist.ReportReason) error {
	// Validate
	err := validate.ValidateFields(api.validator, validate.ValidationMap{"userId": validate.WithTag(userId, "required")})
	if err != nil {
		return err
	}
	err = api.queries.BlockUserFromFeed(ctx, db.BlockUserFromFeedParams{ID: persist.GenerateID(), UserID: userId, Reason: reason})
	if err != nil {
		return err
	}
	// Re-calculate trending feed
	return api.cache.Client().Del(ctx, trendingFeedCacheKey).Err()
}

func (api FeedAPI) UnbanUser(ctx context.Context, userId persist.DBID) error {
	// Validate
	err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userId": validate.WithTag(userId, "required"),
	})
	if err != nil {
		return err
	}
	err = api.queries.UnblockUserFromFeed(ctx, userId)
	if err != nil {
		return err
	}
	// Re-calculate trending feed
	return api.cache.Client().Del(ctx, trendingFeedCacheKey).Err()
}

func (api FeedAPI) GetFeedEventById(ctx context.Context, feedEventID persist.DBID) (*db.FeedEvent, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"feedEventID": validate.WithTag(feedEventID, "required"),
	}); err != nil {
		return nil, err
	}

	event, err := api.loaders.GetEventByIdBatch.Load(feedEventID)
	if err != nil {
		return nil, err
	}

	return &event, nil
}

func (api FeedAPI) GetPostById(ctx context.Context, postID persist.DBID) (*db.Post, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"feedEventID": validate.WithTag(postID, "required"),
	}); err != nil {
		return nil, err
	}

	post, err := api.loaders.GetPostByIdBatch.Load(postID)
	if err != nil {
		return nil, err
	}

	return &post, nil
}

func (api FeedAPI) PostTokens(ctx context.Context, tokenIDs []persist.DBID, mentions []*model.MentionInput, caption, mintURL *string) (persist.DBID, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"tokenIDs": validate.WithTag(tokenIDs, "required"),
		// caption can be null but less than 2000 chars
		"caption": validate.WithTag(caption, "max=2000"),
		"mintURL": validate.WithTag(mintURL, "omitempty,http"),
	}); err != nil {
		return "", err
	}
	actorID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return "", err
	}

	contracts, err := api.queries.GetContractsByTokenIDs(ctx, tokenIDs)
	if err != nil {
		return "", err
	}

	contractIDs := util.MapWithoutError(contracts, func(c db.Contract) persist.DBID {
		return c.ID
	})

	tokens, errs := api.loaders.GetTokenByIdBatch.LoadAll(tokenIDs)
	tokenDefinitionIDs := make([]persist.DBID, 0, len(tokens))
	for i, err := range errs {
		if err == nil {
			tokenDefinitionIDs = append(tokenDefinitionIDs, tokens[i].Token.TokenDefinitionID)
		}
	}

	tx, err := api.repos.BeginTx(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	q := api.queries.WithTx(tx)

	postID, err := q.InsertPost(ctx, db.InsertPostParams{
		ID:          persist.GenerateID(),
		TokenIds:    tokenIDs,
		ContractIds: contractIDs,
		ActorID:     actorID,
		Caption:     util.ToNullStringEmptyNull(util.GetOptionalValue(caption, "")),
		UserMintUrl: util.ToNullStringEmptyNull(util.GetOptionalValue(mintURL, "")),
	})
	if err != nil {
		return "", err
	}

	dbMentions, err := insertMentionsForPost(ctx, mentions, postID, q)
	if err != nil {
		return "", err
	}
	if len(dbMentions) > 0 {
		for _, mention := range dbMentions {
			switch {
			case mention.UserID != "":
				err = event.Dispatch(ctx, db.Event{
					ActorID:        persist.DBIDToNullStr(actorID),
					ResourceTypeID: persist.ResourceTypeUser,
					SubjectID:      mention.UserID,
					PostID:         postID,
					UserID:         mention.UserID,
					Action:         persist.ActionMentionUser,
					MentionID:      mention.ID,
				})
				if err != nil {
					return "", err
				}
			case mention.CommunityID != "":
				err = event.Dispatch(ctx, db.Event{
					ActorID:        persist.DBIDToNullStr(actorID),
					ResourceTypeID: persist.ResourceTypeCommunity,
					SubjectID:      mention.CommunityID,
					PostID:         postID,
					CommunityID:    mention.CommunityID,
					Action:         persist.ActionMentionCommunity,
					MentionID:      mention.ID,
				})
				if err != nil {
					return "", err
				}
			default:
				return "", fmt.Errorf("invalid mention type: %+v", mention)
			}
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return "", err
	}

	err = event.Dispatch(ctx, db.Event{
		ActorID:        persist.DBIDToNullStr(actorID),
		Action:         persist.ActionUserPosted,
		ResourceTypeID: persist.ResourceTypePost,
		UserID:         actorID,
		SubjectID:      postID,
		PostID:         postID,
	})
	if err != nil {
		sentryutil.ReportError(ctx, fmt.Errorf("error dispatching event: %w", err))
		logger.For(ctx).Errorf("error dispatching event: %v", err)
	}

	return postID, nil
}

func (api FeedAPI) ReferralPostToken(ctx context.Context, t persist.TokenIdentifiers, caption, mintURL *string) (persist.DBID, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"token": validate.WithTag(t, "required"),
		// caption can be null but less than 2000 chars
		"caption": validate.WithTag(caption, "max=2000"),
		"mintURL": validate.WithTag(mintURL, "omitempty,http"),
	}); err != nil {
		return "", err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return "", err
	}

	r, err := api.loaders.GetTokenByUserTokenIdentifiersBatch.Load(db.GetTokenByUserTokenIdentifiersBatchParams{
		OwnerID:         userID,
		TokenID:         t.TokenID,
		ContractAddress: t.ContractAddress,
		Chain:           t.Chain,
	})

	var tokenID persist.DBID
	var contractID persist.DBID

	if err == nil {
		// The token is already synced
		tokenID = r.Token.ID
		contractID = r.Contract.ID
	} else if !util.ErrorIs[persist.ErrTokenNotFoundByUserTokenIdentifers](err) {
		// Unexpected error
		return "", err
	} else {
		// The token is not synced, so we need to find it
		synced, err := api.multichainProvider.SyncTokenByUserWalletsAndTokenIdentifiersRetry(ctx, userID, t, retry.Retry{
			MinWait:    2,
			MaxWait:    4,
			MaxRetries: 4,
		})
		if err != nil {
			return "", err
		}

		tokenID = synced.Instance.ID
		contractID = synced.Contract.ID
	}

	postID, err := api.queries.InsertPost(ctx, db.InsertPostParams{
		ID:          persist.GenerateID(),
		TokenIds:    []persist.DBID{tokenID},
		ContractIds: []persist.DBID{contractID},
		ActorID:     userID,
		Caption:     util.ToNullStringEmptyNull(util.GetOptionalValue(caption, "")),
		UserMintUrl: util.ToNullStringEmptyNull(util.GetOptionalValue(mintURL, "")),
	})
	if err != nil {
		return postID, err
	}

	err = event.Dispatch(ctx, db.Event{
		ActorID:        persist.DBIDToNullStr(userID),
		Action:         persist.ActionUserPosted,
		ResourceTypeID: persist.ResourceTypePost,
		UserID:         userID,
		SubjectID:      postID,
		PostID:         postID,
	})
	if err != nil {
		sentryutil.ReportError(ctx, fmt.Errorf("error dispatching event: %w", err))
		logger.For(ctx).Errorf("error dispatching event: %v", err)
	}

	return postID, nil
}

func (api FeedAPI) ReferralPostPreflight(ctx context.Context, t persist.TokenIdentifiers) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"address": validate.WithTag(t.ContractAddress, "required"),
		"tokenID": validate.WithTag(t.TokenID, "required"),
	}); err != nil {
		return err
	}
	userID, _ := getAuthenticatedUserID(ctx)
	return api.taskClient.CreateTaskForPostPreflight(ctx, task.PostPreflightMessage{Token: t, UserID: userID})
}

func insertMentionsForPost(ctx context.Context, mentions []*model.MentionInput, postID persist.DBID, q *db.Queries) ([]db.Mention, error) {
	dbMentions, err := mentionInputsToMentions(ctx, mentions, q)
	if err != nil {
		return nil, err
	}
	result := make([]db.Mention, len(dbMentions))

	for i, m := range dbMentions {
		var user, community sql.NullString
		if m.UserID != "" {
			user = sql.NullString{
				String: m.UserID.String(),
				Valid:  true,
			}
		} else if m.CommunityID != "" {
			community = sql.NullString{
				String: m.CommunityID.String(),
				Valid:  true,
			}
		}
		mid, err := q.InsertPostMention(ctx, db.InsertPostMentionParams{
			ID:        persist.GenerateID(),
			User:      user,
			Community: community,
			PostID:    postID,
			Start:     m.Start,
			Length:    m.Length,
		})
		if err != nil {
			return nil, err
		}

		result[i] = mid
	}

	return result, nil
}

func (api FeedAPI) DeletePostById(ctx context.Context, postID persist.DBID) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"postID": validate.WithTag(postID, "required"),
	}); err != nil {
		return err
	}

	err := api.queries.DeletePostByID(ctx, postID)
	if err != nil {
		return err
	}

	return nil
}

func (api FeedAPI) GetRawEventById(ctx context.Context, eventID persist.DBID) (*db.Event, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"eventID": validate.WithTag(eventID, "required"),
	}); err != nil {
		return nil, err
	}

	event, err := api.queries.GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}

	return &event, nil
}

func (api FeedAPI) PersonalFeed(ctx context.Context, before *string, after *string, first *int, last *int) ([]any, PageInfo, error) {
	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return nil, PageInfo{}, err
	}

	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params TimeIDPagingParams) ([]interface{}, error) {
		keys, err := api.queries.PaginatePersonalFeedByUserID(ctx, db.PaginatePersonalFeedByUserIDParams{
			Follower:      userID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})

		if err != nil {
			return nil, err
		}

		return feedEntityToTypedType(ctx, api.loaders, keys)
	}

	paginator := TimeIDPaginator[any]{
		QueryFunc:  queryFunc,
		CursorFunc: feedCursor,
	}

	return paginator.Paginate(before, after, first, last)
}

func (api FeedAPI) UserFeed(ctx context.Context, userID persist.DBID, before *string, after *string, first *int, last *int) ([]db.Post, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params TimeIDPagingParams) ([]db.Post, error) {
		return api.queries.PaginatePostsByUserID(ctx, db.PaginatePostsByUserIDParams{
			UserID:        userID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})
	}

	countFunc := func() (int, error) {
		c, err := api.queries.CountPostsByUserID(ctx, userID)
		return int(c), err
	}

	cursorFunc := func(p db.Post) (time.Time, persist.DBID, error) {
		return p.CreatedAt, p.ID, nil
	}

	paginator := TimeIDPaginator[db.Post]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	return paginator.Paginate(before, after, first, last)
}

func (api FeedAPI) GlobalFeed(ctx context.Context, before *string, after *string, first *int, last *int) ([]any, PageInfo, error) {
	// Validate
	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}
	viewerID, _ := getAuthenticatedUserID(ctx)

	queryFunc := func(params TimeIDPagingParams) ([]any, error) {
		keys, err := api.queries.PaginateGlobalFeed(ctx, db.PaginateGlobalFeedParams{
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
			ViewerID:      viewerID,
		})

		if err != nil {
			return nil, err
		}

		return feedEntityToTypedType(ctx, api.loaders, keys)
	}

	paginator := TimeIDPaginator[any]{
		QueryFunc:  queryFunc,
		CursorFunc: feedCursor,
	}

	return paginator.Paginate(before, after, first, last)
}

func fetchFeedEntityScores(ctx context.Context, q *db.Queries, viewerID persist.DBID) ([]db.GetFeedEntityScoresRow, error) {
	return q.GetFeedEntityScores(ctx, db.GetFeedEntityScoresParams{
		WindowEnd: time.Now().Add(-time.Minute * time.Duration(feedOpts.LookbackWindow)),
		ViewerID:  viewerID,
		Span:      int32(feedOpts.PostSpan),
	})
}

func (api FeedAPI) paginatorFromCursorStr(ctx context.Context, curStr string) (feedPaginator, error) {
	cur := cursors.NewFeedPositionCursor()
	err := cur.Unpack(curStr)
	if err != nil {
		return feedPaginator{}, err
	}
	paginator := api.paginatorFromCursor(ctx, cur)
	return paginator, nil
}

func (api FeedAPI) paginatorFromCursor(ctx context.Context, c *feedPositionCursor) feedPaginator {
	return api.paginatorWithQuery(c, func(p positionPagingParams) ([]any, error) {
		params := db.GetPostsByIdsPaginateBatchParams{
			PostIds: util.MapWithoutError(c.EntityIDs, func(i persist.DBID) string { return i.String() }),
			// Postgres uses 1-based indexing
			CurAfterPos:  p.CursorAfterPos + 1,
			CurBeforePos: p.CursorBeforePos + 1,
		}
		posts, err := api.loaders.GetPostsByIdsPaginateBatch.Load(params)
		return util.MapWithoutError(posts, func(p db.Post) any { return p }), err
	})
}

func (api FeedAPI) paginatorFromResults(ctx context.Context, c *feedPositionCursor, posts []db.Post) feedPaginator {
	entities := util.MapWithoutError(posts, func(p db.Post) any { return p })
	return api.paginatorWithQuery(c, func(positionPagingParams) ([]any, error) { return entities, nil })
}

func (api FeedAPI) paginatorWithQuery(c *feedPositionCursor, queryF func(positionPagingParams) ([]any, error)) feedPaginator {
	var paginator feedPaginator
	paginator.QueryFunc = queryF
	paginator.CursorFunc = func(node any) (int64, []persist.FeedEntityType, []persist.DBID, error) {
		_, id, err := feedCursor(node)
		return c.Positions[id], c.EntityTypes, c.EntityIDs, err
	}
	return paginator
}

func (api FeedAPI) TrendingFeed(ctx context.Context, before *string, after *string, first *int, last *int) ([]any, PageInfo, error) {
	// Validate
	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	var paginator feedPaginator
	var err error

	if before != nil {
		paginator, err = api.paginatorFromCursorStr(ctx, *before)
		if err != nil {
			return nil, PageInfo{}, err
		}
	} else if after != nil {
		paginator, err = api.paginatorFromCursorStr(ctx, *after)
		if err != nil {
			return nil, PageInfo{}, err
		}
	} else {
		// Not currently paging, so we need to check the cache and possibly re-calculate the feed
		var posts []db.Post

		// Func to calculate the feed. It's only called if the cache is empty.
		cacheCalcFunc := func(ctx context.Context) ([]persist.FeedEntityType, []persist.DBID, error) {
			viewerID, _ := getAuthenticatedUserID(ctx)
			postScores, err := fetchFeedEntityScores(ctx, api.queries, viewerID)
			if err != nil {
				return nil, nil, err
			}

			now := time.Now()

			scored := api.scoreFeedEntities(ctx, feedOpts.FetchSize, postScores, func(i int) float64 {
				e := postScores[i]
				return timeFactor(now.Sub(e.Post.CreatedAt).Minutes(), e.IsGalleryPost) * engagementFactor(float64(e.FeedEntityScore.Interactions))
			})

			postIDs := make([]persist.DBID, len(scored))
			posts = make([]db.Post, len(scored))
			postTypes := make([]persist.FeedEntityType, len(scored))

			for i, e := range scored {
				idx := len(scored) - i - 1
				postIDs[idx] = e.Post.ID
				posts[idx] = e.Post
				postTypes[idx] = persist.FeedEntityType(e.FeedEntityScore.FeedEntityType)
			}

			return postTypes, postIDs, nil
		}

		cache := newFeedCache(api.cache, cacheCalcFunc)

		// Load from cache
		postTypes, postIDs, err := cache.Load(ctx)
		if err != nil {
			return nil, PageInfo{}, err
		}

		// Create cursor from cached data
		cursor := cursors.NewFeedPositionCursor()
		cursor.CurrentPosition = 0
		cursor.EntityTypes = postTypes
		cursor.EntityIDs = postIDs
		cursor.Positions = sliceToMapIndex(postIDs)

		// We already did the work to fetch the posts when re-calculating the feed, so we can just return them here
		if posts != nil {
			paginator = api.paginatorFromResults(ctx, cursor, posts)
		} else {
			paginator = api.paginatorFromCursor(ctx, cursor)
		}
	}

	return paginator.paginate(before, after, first, last)
}

func (api FeedAPI) ForYouFeed(ctx context.Context, before, after *string, first, last *int) ([]any, PageInfo, error) {
	// Validate
	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	// Fallback to trending if no user
	viewerID, _ := getAuthenticatedUserID(ctx)
	if viewerID == "" {
		return api.TrendingFeed(ctx, before, after, first, last)
	}

	var paginator feedPaginator
	var err error

	if before != nil {
		paginator, err = api.paginatorFromCursorStr(ctx, *before)
		if err != nil {
			return nil, PageInfo{}, err
		}
	} else if after != nil {
		paginator, err = api.paginatorFromCursorStr(ctx, *after)
		if err != nil {
			return nil, PageInfo{}, err
		}
	} else {
		// Not currently paging, so we need to check the cache and possibly re-calculate the feed
		postScores, err := fetchFeedEntityScores(ctx, api.queries, viewerID)
		if err != nil {
			return nil, PageInfo{}, err
		}

		now := time.Now()
		engagementScores := make(map[persist.DBID]float64)
		personalizationScores := make(map[persist.DBID]float64)

		for _, e := range postScores {
			age := now.Sub(e.Post.CreatedAt).Minutes()
			engagementScores[e.Post.ID] = scorePost(age, e.IsGalleryPost, e.Post.IsFirstPost, isFreshPost(age), int(e.Streak))
			personalizationScores[e.Post.ID] = engagementScores[e.Post.ID]
			engagementScores[e.Post.ID] *= engagementFactor(float64(e.FeedEntityScore.Interactions))
			personalizationScores[e.Post.ID] *= userpref.For(ctx).RelevanceTo(viewerID, e.FeedEntityScore)
		}

		// Rank by engagement first, then by personalization
		topNByPersonalization := api.scoreFeedEntities(ctx, feedOpts.FetchSize, postScores, func(i int) float64 { return engagementScores[postScores[i].Post.ID] })
		topNByPersonalization = api.scoreFeedEntities(ctx, feedOpts.FetchSize, topNByPersonalization, func(i int) float64 { return personalizationScores[topNByPersonalization[i].Post.ID] })

		// Rank by personalization, then by engagement
		topNByEngagement := api.scoreFeedEntities(ctx, feedOpts.FetchSize, postScores, func(i int) float64 { return personalizationScores[postScores[i].Post.ID] })
		topNByEngagement = api.scoreFeedEntities(ctx, feedOpts.FetchSize, topNByEngagement, func(i int) float64 { return engagementScores[topNByEngagement[i].Post.ID] })

		// Get ranking of both
		seen := make(map[persist.DBID]int)
		combined := make([]db.GetFeedEntityScoresRow, 0)
		engagementRank := make([]float64, 0)
		blendedRank := make([]float64, 0)

		for i, e := range topNByEngagement {
			score := float64(len(topNByEngagement) - i)
			combined = append(combined, e)
			engagementRank = append(engagementRank, score)
			blendedRank = append(blendedRank, score)
			seen[e.Post.ID] = i
		}

		for i, e := range topNByPersonalization {
			score := float64(len(topNByPersonalization) - i)
			if idx, ok := seen[e.Post.ID]; ok {
				blendedRank[idx] = (blendedRank[idx] + score) * 0.5
			} else {
				combined = append(combined, e)
				engagementRank = append(engagementRank, 0)
				// New posts tend to have no engagement, so don't over penalize it by halving it
				blendedRank = append(blendedRank, score)
			}
		}

		interleaved := api.scoreFeedEntities(ctx, feedOpts.FetchSize, combined, func(i int) float64 {
			if combined[i].Post.ActorID == viewerID {
				return engagementRank[i]
			}
			return blendedRank[i]
		})

		recommend.Shuffle(interleaved, 4)

		posts := make([]db.Post, len(interleaved))
		postIDs := make([]persist.DBID, len(interleaved))
		postTypes := make([]persist.FeedEntityType, len(interleaved))
		positions := make(map[persist.DBID]int64, len(interleaved))

		for i, e := range interleaved {
			idx := len(interleaved) - i - 1
			postIDs[idx] = e.Post.ID
			posts[idx] = e.Post
			postTypes[idx] = persist.FeedEntityType(e.FeedEntityScore.FeedEntityType)
			positions[e.Post.ID] = int64(idx)
		}

		cursor := cursors.NewFeedPositionCursor()
		cursor.CurrentPosition = 0
		cursor.EntityTypes = postTypes
		cursor.EntityIDs = postIDs
		cursor.Positions = positions

		paginator = api.paginatorFromResults(ctx, cursor, posts)
	}

	return paginator.paginate(before, after, first, last)
}

func (api FeedAPI) TrendingUsers(ctx context.Context, report model.Window) ([]db.User, error) {
	ttl := time.Hour

	// Reports that span a week or greater are calculated once every 24 hours rather than once an hour.
	if report.Duration > 7*24*time.Hour {
		ttl *= 24
	}

	calcFunc := func(ctx context.Context) ([]persist.DBID, error) {
		return api.queries.GetAllTimeTrendingUserIDs(ctx, 24)
	}

	if report.Name != "ALL_TIME" {
		calcFunc = func(ctx context.Context) ([]persist.DBID, error) {
			return api.queries.GetWindowedTrendingUserIDs(ctx, db.GetWindowedTrendingUserIDsParams{
				WindowEnd: time.Now().Add(-time.Duration(report.Duration)),
				Limit:     24,
			})
		}
	}

	l := newDBIDCache(redis.FeedCache, "trending:users:"+report.Name, ttl, calcFunc)

	ids, err := l.Load(ctx)
	if err != nil {
		return nil, err
	}

	asStr, _ := util.Map(ids, func(id persist.DBID) (string, error) {
		return id.String(), nil
	})

	return api.queries.GetTrendingUsersByIDs(ctx, asStr)
}

func feedCursor(i any) (time.Time, persist.DBID, error) {
	switch row := i.(type) {
	case db.FeedEvent:
		return row.EventTime, row.ID, nil
	case db.Post:
		return row.CreatedAt, row.ID, nil
	}
	return time.Time{}, "", fmt.Errorf("node is not a feed entity: %T", i)
}

func feedEntityToTypedType(ctx context.Context, d *dataloader.Loaders, entities []db.FeedEntity) ([]any, error) {
	idTypes := make([]persist.FeedEntityType, len(entities))
	entityIDs := make([]persist.DBID, len(entities))
	for i := 0; i < len(entities); i++ {
		idTypes[i] = persist.FeedEntityType(entities[i].FeedEntityType)
		entityIDs[i] = entities[i].ID
	}
	return loadFeedEntities(ctx, d, idTypes, entityIDs)
}

func loadFeedEntities(ctx context.Context, d *dataloader.Loaders, typs []persist.FeedEntityType, ids []persist.DBID) ([]any, error) {
	if len(typs) != len(ids) {
		panic("length of types and ids must be equal")
	}

	entities := make([]any, len(ids))
	errored := make([]int, 0)
	idToPosition := make(map[persist.DBID]int, len(ids))
	eventsFetch := make([]persist.DBID, 0, len(ids))
	postsFetch := make([]persist.DBID, 0, len(ids))
	eventsDone := make(chan bool)
	postsDone := make(chan bool)
	eventsErr := make(chan int)
	postsErr := make(chan int)

	for i := 0; i < len(typs); i++ {
		id := ids[i]
		idToPosition[id] = i
		switch persist.FeedEntityType(typs[i]) {
		case persist.FeedEventTypeTag:
			eventsFetch = append(eventsFetch, id)
		case persist.PostTypeTag:
			postsFetch = append(postsFetch, id)
		default:
			logger.For(ctx).Warnf("unknown feed entity type %d", typs[i])
		}
	}

	go func() {
		batchResults, batchErrs := d.GetEventByIdBatch.LoadAll(eventsFetch)
		for i := 0; i < len(batchResults); i++ {
			pos := idToPosition[eventsFetch[i]]
			err := batchErrs[i]
			entities[pos] = batchResults[i]
			if err != nil && !util.ErrorIs[persist.ErrFeedEventNotFoundByID](err) {
				logger.For(ctx).Errorf("failed to fetch event %s: %s", eventsFetch[i], err)
				eventsErr <- pos
			}
		}
		close(eventsDone)
		close(eventsErr)
	}()

	go func() {
		batchResults, batchErrs := d.GetPostByIdBatch.LoadAll(postsFetch)
		for i := 0; i < len(batchResults); i++ {
			pos := idToPosition[postsFetch[i]]
			err := batchErrs[i]
			entities[pos] = batchResults[i]
			if err != nil && !util.ErrorIs[persist.ErrPostNotFoundByID](err) {
				logger.For(ctx).Errorf("failed to fetch post %s: %s", postsFetch[i], err)
				postsErr <- pos
			}
		}
		close(postsDone)
		close(postsErr)
	}()

	for pos := range eventsErr {
		errored = append(errored, pos)
	}
	for pos := range postsErr {
		errored = append(errored, pos)
	}

	<-eventsDone
	<-postsDone

	// Sort in descending order
	sort.Slice(errored, func(i, j int) bool { return errored[i] > errored[j] })

	// Filter out errored entities
	for _, pos := range errored {
		if pos == 0 {
			entities = entities[1:]
			continue
		}
		if pos == len(entities)-1 {
			entities = entities[:pos]
			continue
		}
		entities = append(entities[:pos], entities[pos+1:]...)
	}

	return entities, nil
}

func (api FeedAPI) scoreFeedEntities(ctx context.Context, n int, trendData []db.GetFeedEntityScoresRow, scoreF func(i int) float64) []db.GetFeedEntityScoresRow {
	h := make(heap[entityScore], 0)

	scores := make([]entityScore, len(trendData))

	for i, event := range trendData {
		score := scoreF(i)
		scores[i] = entityScore{v: event, s: score}
	}

	for _, node := range scores {
		// Add first n items in the heap
		if h.Len() < n {
			heappkg.Push(&h, node)
			continue
		}
		// If the score is greater than the smallest score in the heap, replace it
		if node.s > h[0].s {
			heappkg.Pop(&h)
			heappkg.Push(&h, node)
		}
	}

	scoredEntities := make([]db.GetFeedEntityScoresRow, h.Len())

	// Pop returns the smallest score first, so we reverse the order
	// such that the highest score is first
	i := h.Len() - 1
	for h.Len() > 0 {
		node := heappkg.Pop(&h)
		scoredEntities[i] = node.(entityScore).v
		i--
	}

	return scoredEntities
}

func isFreshPost(age float64) bool {
	return age < feedOpts.FreshnessWindow
}

func scorePost(age float64, isGallery, isFirstPost, isFresh bool, streak int) (s float64) {
	s = timeFactor(age, isGallery)
	if streak > feedOpts.StreakThreshold {
		s *= 1 / math.Pow(math.E, feedOpts.StreakFactor*float64(streak-feedOpts.StreakThreshold+1))
	}
	if isFirstPost && isFresh {
		s *= feedOpts.FirstPostFactor
	}
	if !isFirstPost && streak < feedOpts.StreakThreshold && isFresh {
		s *= feedOpts.FreshnessFactor
	}
	return s
}

func timeFactor(age float64, isGalleryPost bool) float64 {
	if isGalleryPost {
		return math.Pow(2, -(age / lerp(age, feedOpts.PostHalfLife, feedOpts.GalleryPostHalfLife, feedOpts.GalleryDecayPeriod)))
	}
	return math.Pow(2, -(age / feedOpts.PostHalfLife))
}

// lerp returns a linear interpolation between s and e (clamped to e) based on age
// period controls the time it takes to reach e from s
func lerp(age, s, e, period float64) float64 {
	return math.Min(e, s+((e-s)/period)*age)
}

func engagementFactor(interactions float64) float64 {
	// Add 2 because log(0) => undefined and log(1) => 0 and returning 0 will cancel out
	// the effect of other terms this term may get multiplied with
	return math.Log2(2 + interactions)
}

type entityScore struct {
	v db.GetFeedEntityScoresRow
	s float64
}

func (n entityScore) Less(a any) bool {
	other, ok := a.(entityScore)
	if !ok {
		return false
	}
	return n.s < other.s
}

type lt interface{ Less(j any) bool }
type heap[T lt] []T

func (h heap[T]) Len() int           { return len(h) }
func (h heap[T]) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *heap[T]) Push(s any)        { *h = append(*h, s.(T)) }
func (h heap[T]) Less(i, j int) bool { return h[i].Less(h[j]) }

func (h *heap[T]) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

type feedPaginator struct {
	QueryFunc  func(params positionPagingParams) ([]any, error)
	CursorFunc func(node any) (pos int64, feedEntityType []persist.FeedEntityType, ids []persist.DBID, err error)
}

func (p *feedPaginator) paginate(before, after *string, first, last *int) ([]any, PageInfo, error) {
	args := positionPagingParams{
		CursorBeforePos: int32(defaultCursorBeforePosition),
		CursorAfterPos:  int32(defaultCursorAfterPosition),
	}

	beforeCur := cursors.NewFeedPositionCursor()
	afterCur := cursors.NewFeedPositionCursor()

	if before != nil {
		if err := beforeCur.Unpack(*before); err != nil {
			return nil, PageInfo{}, err
		}
		args.CursorBeforePos = int32(beforeCur.CurrentPosition)
	}

	if after != nil {
		if err := afterCur.Unpack(*after); err != nil {
			return nil, PageInfo{}, err
		}
		args.CursorAfterPos = int32(afterCur.CurrentPosition)
	}

	results, err := p.QueryFunc(args)
	if err != nil {
		return nil, PageInfo{}, err
	}

	return pageFrom(results, nil, newFeedPositionCursor(p.CursorFunc), before, after, first, last)
}

type feedCache struct {
	*redis.LazyCache
	CalcFunc func(context.Context) ([]persist.FeedEntityType, []persist.DBID, error)
}

func newFeedCache(cache *redis.Cache, f func(context.Context) ([]persist.FeedEntityType, []persist.DBID, error)) *feedCache {
	return &feedCache{
		LazyCache: &redis.LazyCache{
			Cache: cache,
			Key:   trendingFeedCacheKey,
			TTL:   time.Minute * 10,
			CalcFunc: func(ctx context.Context) ([]byte, error) {
				types, ids, err := f(ctx)
				if err != nil {
					return nil, err
				}
				cur := cursors.NewFeedPositionCursor()
				cur.EntityTypes = types
				cur.EntityIDs = ids
				b, err := cur.Pack()
				return []byte(b), err
			},
		},
	}
}

func (f feedCache) Load(ctx context.Context) ([]persist.FeedEntityType, []persist.DBID, error) {
	b, err := f.LazyCache.Load(ctx)
	if err != nil {
		return nil, nil, err
	}
	cur := cursors.NewFeedPositionCursor()
	err = cur.Unpack(string(b))
	return cur.EntityTypes, cur.EntityIDs, err
}
