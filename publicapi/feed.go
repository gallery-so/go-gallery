package publicapi

import (
	"container/heap"
	"context"
	"database/sql"
	"fmt"
	"math"
	sortpkg "sort"
	"sync"
	"time"

	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/validate"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"

	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"

	"github.com/mikeydub/go-gallery/service/recommend"
	"github.com/mikeydub/go-gallery/service/recommend/userpref"

	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
	"github.com/mikeydub/go-gallery/util/sort"
)

const trendingFeedCacheKey = "trending:feedEvents:all"

var feedOpts = struct {
	FreshnessFactor     float64 // extra weight added to a new post
	FirstPostFactor     float64 // extra weight added to a first post
	LookbackWindow      float64 // how far back to look for feed events
	FreshnessWindow     float64 // how long a post is considered new
	PostHalfLife        float64 // controls the decay rate of posts
	GalleryPostHalfLife float64 // controls the decay rate of Gallery posts
	GalleryDecayPeriod  float64 // time it takes for a Gallery post to reach GalleryPostHalfLife from PostHalfLife
}{
	FreshnessFactor:     2.0,
	FirstPostFactor:     2.0,
	LookbackWindow:      time.Duration(4 * 24 * time.Hour).Minutes(),
	FreshnessWindow:     time.Duration(6 * time.Hour).Minutes(),
	PostHalfLife:        time.Duration(6 * time.Hour).Minutes(),
	GalleryPostHalfLife: time.Duration(10 * time.Hour).Minutes(),
	GalleryDecayPeriod:  time.Duration(4 * 24 * time.Hour).Minutes(),
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
		// caption can be null but less than 600 chars
		"caption": validate.WithTag(caption, "max=600"),
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
		Caption:     util.ToSQLNullString(caption),
		UserMintUrl: util.ToSQLNullString(mintURL),
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
			case mention.ContractID != "":
				err = event.Dispatch(ctx, db.Event{
					ActorID:        persist.DBIDToNullStr(actorID),
					ResourceTypeID: persist.ResourceTypeContract,
					SubjectID:      mention.ContractID,
					PostID:         postID,
					ContractID:     mention.ContractID,
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

	creators, _ := api.loaders.GetContractCreatorsByIds.LoadAll(util.StringersToStrings(contractIDs))
	for _, creator := range creators {
		if creator.CreatorUserID == "" {
			continue
		}
		err = event.Dispatch(ctx, db.Event{
			ActorID:        persist.DBIDToNullStr(actorID),
			Action:         persist.ActionUserPostedYourWork,
			ResourceTypeID: persist.ResourceTypeContract,
			UserID:         creator.CreatorUserID,
			SubjectID:      creator.ContractID,
			PostID:         postID,
			ContractID:     creator.ContractID,
		})
		if err != nil {
			sentryutil.ReportError(ctx, fmt.Errorf("error dispatching event: %w", err))
			logger.For(ctx).Errorf("error dispatching event: %v", err)
		}
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

	count, err := api.queries.CountPostsByUserID(ctx, actorID)
	if err != nil {
		return "", err
	}

	if count == 1 {
		err = event.Dispatch(ctx, db.Event{
			ActorID:        persist.DBIDToNullStr(actorID),
			Action:         persist.ActionUserPostedFirstPost,
			ResourceTypeID: persist.ResourceTypePost,
			UserID:         actorID,
			SubjectID:      postID,
			PostID:         postID,
		})
		if err != nil {
			sentryutil.ReportError(ctx, fmt.Errorf("error dispatching event: %w", err))
			logger.For(ctx).Errorf("error dispatching event: %v", err)
		}
	}

	return postID, nil
}

func (api FeedAPI) ReferralPostToken(ctx context.Context, t persist.TokenIdentifiers, caption, mintURL *string) (persist.DBID, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"token": validate.WithTag(t, "required"),
		// caption can be null but less than 600 chars
		"caption": validate.WithTag(caption, "max=600"),
		"mintURL": validate.WithTag(mintURL, "omitempty,http"),
	}); err != nil {
		return "", err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return "", err
	}

	user, err := api.repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}

	r, err := api.loaders.GetTokenByUserTokenIdentifiersBatch.Load(db.GetTokenByUserTokenIdentifiersBatchParams{
		OwnerID:         user.ID,
		TokenID:         t.TokenID,
		ContractAddress: t.ContractAddress,
		Chain:           t.Chain,
	})

	// The token is already synced
	if err == nil {
		postID, err := api.queries.InsertPost(ctx, db.InsertPostParams{
			ID:          persist.GenerateID(),
			TokenIds:    []persist.DBID{r.Token.ID},
			ContractIds: []persist.DBID{r.Contract.ID},
			ActorID:     user.ID,
			Caption:     util.ToSQLNullString(caption),
			UserMintUrl: util.ToSQLNullString(mintURL),
		})
		if err != nil {
			return postID, err
		}

		creator, _ := api.loaders.GetContractCreatorsByIds.Load(r.Contract.ID.String())
		if creator.CreatorUserID != "" {
			err = event.Dispatch(ctx, db.Event{
				ActorID:        persist.DBIDToNullStr(userID),
				Action:         persist.ActionUserPostedYourWork,
				ResourceTypeID: persist.ResourceTypeContract,
				UserID:         creator.CreatorUserID,
				SubjectID:      creator.ContractID,
				PostID:         postID,
				ContractID:     creator.ContractID,
			})
			if err != nil {
				sentryutil.ReportError(ctx, fmt.Errorf("error dispatching event: %w", err))
				logger.For(ctx).Errorf("error dispatching event: %v", err)
			}
		}

		err = event.Dispatch(ctx, db.Event{
			ActorID:        persist.DBIDToNullStr(user.ID),
			Action:         persist.ActionUserPosted,
			ResourceTypeID: persist.ResourceTypePost,
			UserID:         user.ID,
			SubjectID:      postID,
			PostID:         postID,
		})

		return postID, err
	}

	// Unexpected error
	if err != nil && !util.ErrorAs[persist.ErrTokenNotFoundByUserTokenIdentifers](err) {
		return "", err
	}

	// The token is not synced, so we need to find it
	synced, err := api.multichainProvider.SyncTokenByUserWalletsAndTokenIdentifiersRetry(ctx, user, t, retry.Retry{
		Base:  2,
		Cap:   4,
		Tries: 4,
	})
	if err != nil {
		return "", err
	}

	postID, err := api.queries.InsertPost(ctx, db.InsertPostParams{
		ID:          persist.GenerateID(),
		TokenIds:    []persist.DBID{synced.Instance.ID},
		ContractIds: []persist.DBID{synced.Contract.ID},
		ActorID:     user.ID,
		Caption:     util.ToSQLNullString(caption),
		UserMintUrl: util.ToSQLNullString(mintURL),
	})
	if err != nil {
		return postID, err
	}

	creator, _ := api.loaders.GetContractCreatorsByIds.Load(synced.Contract.ID.String())
	if creator.CreatorUserID != "" {
		err = event.Dispatch(ctx, db.Event{
			ActorID:        persist.DBIDToNullStr(userID),
			Action:         persist.ActionUserPostedYourWork,
			ResourceTypeID: persist.ResourceTypeContract,
			UserID:         creator.CreatorUserID,
			SubjectID:      creator.ContractID,
			PostID:         postID,
			ContractID:     creator.ContractID,
		})
		if err != nil {
			sentryutil.ReportError(ctx, fmt.Errorf("error dispatching event: %w", err))
			logger.For(ctx).Errorf("error dispatching event: %v", err)
		}
	}

	err = event.Dispatch(ctx, db.Event{
		ActorID:        persist.DBIDToNullStr(user.ID),
		Action:         persist.ActionUserPosted,
		ResourceTypeID: persist.ResourceTypePost,
		UserID:         user.ID,
		SubjectID:      postID,
		PostID:         postID,
	})
	if err != nil {
		sentryutil.ReportError(ctx, fmt.Errorf("error dispatching event: %w", err))
		logger.For(ctx).Errorf("error dispatching event: %v", err)
	}
	count, err := api.queries.CountPostsByUserID(ctx, user.ID)
	if err != nil {
		return "", err
	}

	if count == 1 {
		err = event.Dispatch(ctx, db.Event{
			ActorID:        persist.DBIDToNullStr(user.ID),
			Action:         persist.ActionUserPostedFirstPost,
			ResourceTypeID: persist.ResourceTypePost,
			UserID:         user.ID,
			SubjectID:      postID,
			PostID:         postID,
		})
		if err != nil {
			sentryutil.ReportError(ctx, fmt.Errorf("error dispatching event: %w", err))
			logger.For(ctx).Errorf("error dispatching event: %v", err)
		}
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
		var user, contract sql.NullString
		if m.UserID != "" {
			user = sql.NullString{
				String: m.UserID.String(),
				Valid:  true,
			}
		} else if m.ContractID != "" {
			contract = sql.NullString{
				String: m.ContractID.String(),
				Valid:  true,
			}
		}
		mid, err := q.InsertPostMention(ctx, db.InsertPostMentionParams{
			ID:       persist.GenerateID(),
			User:     user,
			Contract: contract,
			PostID:   postID,
			Start:    m.Start,
			Length:   m.Length,
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

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
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

	paginator := timeIDPaginator[any]{
		QueryFunc:  queryFunc,
		CursorFunc: feedCursor,
	}

	return paginator.paginate(before, after, first, last)
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

	queryFunc := func(params timeIDPagingParams) ([]db.Post, error) {
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

	paginator := timeIDPaginator[db.Post]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	return paginator.paginate(before, after, first, last)
}

func (api FeedAPI) GlobalFeed(ctx context.Context, before *string, after *string, first *int, last *int) ([]any, PageInfo, error) {
	// Validate
	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}
	viewerID, _ := getAuthenticatedUserID(ctx)

	queryFunc := func(params timeIDPagingParams) ([]any, error) {
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

	paginator := timeIDPaginator[any]{
		QueryFunc:  queryFunc,
		CursorFunc: feedCursor,
	}

	return paginator.paginate(before, after, first, last)
}

func fetchFeedEntityScores(ctx context.Context, q *db.Queries, viewerID persist.DBID) (map[persist.DBID]db.GetFeedEntityScoresRow, error) {
	scores, err := q.GetFeedEntityScores(ctx, db.GetFeedEntityScoresParams{
		WindowEnd: time.Now().Add(-time.Minute * time.Duration(feedOpts.LookbackWindow)),
		ViewerID:  viewerID,
	})
	if err != nil {
		return nil, err
	}
	scoreMap := make(map[persist.DBID]db.GetFeedEntityScoresRow)
	for _, s := range scores {
		scoreMap[s.Post.ID] = s
	}
	return scoreMap, nil
}

func (api FeedAPI) paginatorFromCursorStr(ctx context.Context, c string, q *db.Queries) (feedPaginator, error) {
	cur := cursors.NewFeedPositionCursor()
	err := cur.Unpack(c)
	if err != nil {
		return feedPaginator{}, err
	}
	paginator := api.paginatorFromCursor(ctx, cur, q)
	return paginator, nil
}

func (api FeedAPI) paginatorFromCursor(ctx context.Context, c *feedPositionCursor, q *db.Queries) feedPaginator {
	return api.paginatorWithQuery(c, func(p positionPagingParams) ([]any, error) {
		mapped := util.MapWithoutError(c.EntityIDs, func(i persist.DBID) string { return i.String() })
		posts, err := q.GetPostsByIdsPaginate(ctx, db.GetPostsByIdsPaginateParams{
			PostIds: mapped,
			// Postgres uses 1-based indexing
			CurAfterPos:  p.CursorAfterPos + 1,
			CurBeforePos: p.CursorBeforePos + 1,
		})
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
		paginator, err = api.paginatorFromCursorStr(ctx, *before, api.queries)
		if err != nil {
			return nil, PageInfo{}, err
		}
	} else if after != nil {
		paginator, err = api.paginatorFromCursorStr(ctx, *after, api.queries)
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

			scores := util.MapWithoutError(util.MapValues(postScores), func(s db.GetFeedEntityScoresRow) db.FeedEntityScore { return s.FeedEntityScore })
			scored := api.scoreFeedEntities(ctx, 128, scores, func(e db.FeedEntityScore) float64 {
				return timeFactor(now.Sub(e.CreatedAt).Minutes(), postScores[e.ID].IsGalleryPost) * engagementFactor(float64(e.Interactions))
			})

			postIDs := make([]persist.DBID, len(scored))
			posts = make([]db.Post, len(scored))
			postTypes := make([]persist.FeedEntityType, len(scored))

			for i, post := range scored {
				idx := len(scored) - i - 1
				postIDs[idx] = post.ID
				posts[idx] = postScores[post.ID].Post
				postTypes[idx] = persist.FeedEntityType(post.FeedEntityType)
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
		cursor.Positions = util.SliceToMapIndex(postIDs)

		// We already did the work to fetch the posts when re-calculating the feed, so we can just return them here
		if posts != nil {
			paginator = api.paginatorFromResults(ctx, cursor, posts)
		} else {
			paginator = api.paginatorFromCursor(ctx, cursor, api.queries)
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
		paginator, err = api.paginatorFromCursorStr(ctx, *before, api.queries)
		if err != nil {
			return nil, PageInfo{}, err
		}
	} else if after != nil {
		paginator, err = api.paginatorFromCursorStr(ctx, *after, api.queries)
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
		scores := util.MapWithoutError(util.MapValues(postScores), func(s db.GetFeedEntityScoresRow) db.FeedEntityScore { return s.FeedEntityScore })
		engagementScores := make(map[persist.DBID]float64)
		personalizationScores := make(map[persist.DBID]float64)

		for _, e := range postScores {
			engagementScores[e.Post.ID] = scorePost(e.Post, now, e.IsGalleryPost, float64(e.FeedEntityScore.Interactions))
			personalizationScores[e.Post.ID] = engagementScores[e.Post.ID]
			engagementScores[e.Post.ID] *= engagementFactor(float64(e.FeedEntityScore.Interactions))
			if !e.IsGalleryPost {
				personalizationScores[e.Post.ID] *= userpref.For(ctx).RelevanceTo(viewerID, e.FeedEntityScore)
			}
		}

		// Rank by engagement first, then by personalization
		topNByPersonalization := api.scoreFeedEntities(ctx, 128, scores, func(e db.FeedEntityScore) float64 { return engagementScores[e.ID] })
		topNByPersonalization = api.scoreFeedEntities(ctx, 128, topNByPersonalization, func(e db.FeedEntityScore) float64 { return personalizationScores[e.ID] })

		// Rank by personalization, then by engagement
		topNByEngagement := api.scoreFeedEntities(ctx, 128, scores, func(e db.FeedEntityScore) float64 { return personalizationScores[e.ID] })
		topNByEngagement = api.scoreFeedEntities(ctx, 128, topNByEngagement, func(e db.FeedEntityScore) float64 { return engagementScores[e.ID] })

		// Get ranking of both
		seen := make(map[persist.DBID]bool)
		combined := make([]db.FeedEntityScore, 0)
		engagementRank := make(map[persist.DBID]int)
		personalizationRank := make(map[persist.DBID]int)

		for i, e := range topNByEngagement {
			engagementRank[e.ID] = len(topNByEngagement) - i
			if !seen[e.ID] {
				combined = append(combined, e)
				seen[e.ID] = true
			}
		}

		for i, e := range topNByPersonalization {
			personalizationRank[e.ID] = len(topNByPersonalization) - i
			if !seen[e.ID] {
				combined = append(combined, e)
				seen[e.ID] = true
			}
		}

		// Score based on the average of the two rankings
		interleaved := api.scoreFeedEntities(ctx, 128, combined, func(e db.FeedEntityScore) float64 {
			if e.ActorID == viewerID {
				return float64(engagementRank[e.ID])
			}
			return float64(engagementRank[e.ID]+personalizationRank[e.ID]) / 2.0
		})

		recommend.Shuffle(interleaved, 4)

		posts := make([]db.Post, len(interleaved))
		postIDs := make([]persist.DBID, len(interleaved))
		postTypes := make([]persist.FeedEntityType, len(interleaved))

		for i, e := range interleaved {
			idx := len(interleaved) - i - 1
			postIDs[idx] = e.ID
			posts[idx] = postScores[e.ID].Post
			postTypes[idx] = persist.FeedEntityType(e.FeedEntityType)
		}

		cursor := cursors.NewFeedPositionCursor()
		cursor.CurrentPosition = 0
		cursor.EntityTypes = postTypes
		cursor.EntityIDs = postIDs
		cursor.Positions = util.SliceToMapIndex(postIDs)

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

	l := newDBIDCache(api.cache, "trending:users:"+report.Name, ttl, calcFunc)

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
			if err != nil && !util.ErrorAs[persist.ErrFeedEventNotFoundByID](err) {
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
			if err != nil && !util.ErrorAs[persist.ErrPostNotFoundByID](err) {
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
	sortpkg.Slice(errored, func(i, j int) bool { return errored[i] > errored[j] })

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

type entityScore struct {
	v db.FeedEntityScore
	s float64
}

func (n entityScore) Less(a any) bool {
	other, ok := a.(entityScore)
	if !ok {
		return false
	}
	return n.s < other.s
}

func (api FeedAPI) scoreFeedEntities(ctx context.Context, n int, trendData []db.FeedEntityScore, scoreF func(db.FeedEntityScore) float64) []db.FeedEntityScore {
	h := make(sort.Heap[entityScore], 0)

	var wg sync.WaitGroup

	scores := make([]entityScore, len(trendData))

	for i, event := range trendData {
		i := i
		event := event
		wg.Add(1)
		go func() {
			defer wg.Done()
			score := scoreF(event)
			scores[i] = entityScore{v: event, s: score}
		}()
	}

	wg.Wait()

	for _, node := range scores {
		// Add first n items in the heap
		if h.Len() < n {
			heap.Push(&h, node)
			continue
		}
		// If the score is greater than the smallest score in the heap, replace it
		if node.s > h[0].s {
			heap.Pop(&h)
			heap.Push(&h, node)
		}
	}

	scoredEntities := make([]db.FeedEntityScore, h.Len())

	// Pop returns the smallest score first, so we reverse the order
	// such that the highest score is first
	i := h.Len() - 1
	for h.Len() > 0 {
		node := heap.Pop(&h)
		scoredEntities[i] = node.(entityScore).v
		i--
	}

	return scoredEntities
}

func scorePost(p db.Post, t time.Time, isGallery bool, interactions float64) (s float64) {
	age := t.Sub(p.CreatedAt).Minutes()
	s = timeFactor(age, isGallery)
	if age < feedOpts.FreshnessWindow {
		s *= feedOpts.FreshnessFactor
	}
	if p.IsFirstPost {
		s *= feedOpts.FirstPostFactor
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
		args.IDs = beforeCur.EntityIDs
	}

	if after != nil {
		if err := afterCur.Unpack(*after); err != nil {
			return nil, PageInfo{}, err
		}
		args.CursorAfterPos = int32(afterCur.CurrentPosition)
		args.IDs = afterCur.EntityIDs
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
