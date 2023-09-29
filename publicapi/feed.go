package publicapi

import (
	heappkg "container/heap"
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	gcptasks "cloud.google.com/go/cloudtasks/apiv2"
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
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
	"github.com/mikeydub/go-gallery/validate"
)

const tHalf = math.Ln2 / 0.002 // half-life of approx 6 hours

var feedLookback = time.Duration(7 * 24 * time.Hour)

type FeedAPI struct {
	repos              *postgres.Repositories
	queries            *db.Queries
	loaders            *dataloader.Loaders
	validator          *validator.Validate
	ethClient          *ethclient.Client
	cache              *redis.Cache
	taskClient         *gcptasks.Client
	multichainProvider *multichain.Provider
}

func (api FeedAPI) BlockUser(ctx context.Context, userId persist.DBID, action persist.Action) error {
	// Validate
	err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userId": validate.WithTag(userId, "required"),
		"action": validate.WithTag(action, "required"),
	})

	if err != nil {
		return err
	}

	return api.queries.BlockUserFromFeed(ctx, db.BlockUserFromFeedParams{
		ID:     persist.GenerateID(),
		UserID: userId,
		Action: action,
	})

}

func (api FeedAPI) UnBlockUser(ctx context.Context, userId persist.DBID) error {
	// Validate
	err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userId": validate.WithTag(userId, "required"),
	})

	if err != nil {
		return err
	}

	return api.queries.UnblockUserFromFeed(ctx, userId)

}

func (api FeedAPI) GetFeedEventById(ctx context.Context, feedEventID persist.DBID) (*db.FeedEvent, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"feedEventID": validate.WithTag(feedEventID, "required"),
	}); err != nil {
		return nil, err
	}

	event, err := api.loaders.FeedEventByFeedEventID.Load(feedEventID)
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

	post, err := api.loaders.PostByPostID.Load(postID)
	if err != nil {
		return nil, err
	}

	return &post, nil
}

func (api FeedAPI) PostTokens(ctx context.Context, tokenIDs []persist.DBID, caption *string) (persist.DBID, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"tokenIDs": validate.WithTag(tokenIDs, "required"),
		// caption can be null but less than 600 chars
		"caption": validate.WithTag(caption, "max=600"),
	}); err != nil {
		return "", err
	}
	actorID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return "", err
	}

	var c sql.NullString
	if caption != nil {
		c = sql.NullString{
			String: *caption,
			Valid:  true,
		}
	}

	contracts, err := api.queries.GetContractsByTokenIDs(ctx, tokenIDs)
	if err != nil {
		return "", err
	}

	contractIDs, _ := util.Map(contracts, func(c db.Contract) (persist.DBID, error) {
		return c.ID, nil
	})

	postID, err := api.queries.InsertPost(ctx, db.InsertPostParams{
		ID:          persist.GenerateID(),
		TokenIds:    tokenIDs,
		ContractIds: contractIDs,
		ActorID:     actorID,
		Caption:     c,
	})
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

	return postID, err
}

func (api FeedAPI) ReferralPostToken(ctx context.Context, t persist.TokenIdentifiers, caption *string) (persist.DBID, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"token": validate.WithTag(t, "required"),
		// caption can be null but less than 600 chars
		"caption": validate.WithTag(caption, "max=600"),
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

	var c sql.NullString

	if caption != nil {
		c = sql.NullString{
			String: *caption,
			Valid:  true,
		}
	}

	token, err := api.loaders.TokenByUserTokenIdentifiers.Load(db.GetTokenByUserTokenIdentifiersBatchParams{
		OwnerID:         user.ID,
		TokenID:         t.TokenID,
		ContractAddress: t.ContractAddress,
		Chain:           t.Chain,
	})

	// The token is already synced
	if err == nil {
		contract, err := api.queries.GetContractByID(ctx, token.Contract)
		if err != nil {
			return "", err
		}

		postID, err := api.queries.InsertPost(ctx, db.InsertPostParams{
			ID:          persist.GenerateID(),
			TokenIds:    []persist.DBID{token.ID},
			ContractIds: []persist.DBID{contract.ID},
			ActorID:     user.ID,
			Caption:     c,
		})
		if err != nil {
			return postID, err
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
		TokenIds:    []persist.DBID{synced.ID},
		ContractIds: []persist.DBID{synced.Contract},
		ActorID:     user.ID,
		Caption:     c,
	})
	if err != nil {
		return postID, err
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

func (api FeedAPI) ReferralPostPreflight(ctx context.Context, t persist.TokenIdentifiers) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"address": validate.WithTag(t.ContractAddress, "required"),
		"tokenID": validate.WithTag(t.TokenID, "required"),
	}); err != nil {
		return err
	}
	userID, _ := getAuthenticatedUserID(ctx)
	return task.CreateTaskForPostPreflight(ctx, task.PostPreflightMessage{Token: t, UserID: userID}, api.taskClient)
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

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: feedCursor,
	}

	return paginator.paginate(before, after, first, last)
}

func (api FeedAPI) UserFeed(ctx context.Context, userID persist.DBID, before *string, after *string,
	first *int, last *int) ([]any, PageInfo, error) {
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
		keys, err := api.queries.PaginateUserFeedByUserID(ctx, db.PaginateUserFeedByUserIDParams{
			OwnerID:        userID,
			Limit:          params.Limit,
			CurBeforeTime:  params.CursorBeforeTime,
			CurBeforeID:    params.CursorBeforeID,
			CurAfterTime:   params.CursorAfterTime,
			CurAfterID:     params.CursorAfterID,
			PagingForward:  params.PagingForward,
			PostEntityType: int32(persist.PostTypeTag),
		})
		if err != nil {
			return nil, err
		}

		return feedEntityToTypedType(ctx, api.loaders, keys)
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: feedCursor,
	}

	return paginator.paginate(before, after, first, last)
}

func (api FeedAPI) GlobalFeed(ctx context.Context, before *string, after *string, first *int, last *int) ([]any, PageInfo, error) {
	// Validate
	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		keys, err := api.queries.PaginateGlobalFeed(ctx, db.PaginateGlobalFeedParams{
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

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: feedCursor,
	}

	return paginator.paginate(before, after, first, last)
}

func fetchFeedEntityScores(ctx context.Context, queries *db.Queries, excludeUserID persist.DBID) (map[persist.DBID]db.GetFeedEntityScoresRow, error) {
	params := db.GetFeedEntityScoresParams{
		IncludeViewer: true,
		WindowEnd:     time.Now().Add(-feedLookback),
	}
	if excludeUserID != "" {
		params.IncludeViewer = false
		params.ViewerID = excludeUserID
	}
	scores, err := queries.GetFeedEntityScores(ctx, params)
	if err != nil {
		return nil, err
	}
	scoreMap := make(map[persist.DBID]db.GetFeedEntityScoresRow)
	for _, s := range scores {
		scoreMap[s.Post.ID] = s
	}
	return scoreMap, nil
}

func newPaginatorFromCursor(ctx context.Context, cur string, q *db.Queries) (paginator feedPaginator, err error) {
	queryF := func(postIDs []persist.DBID) ([]db.Post, error) {
		ids := util.MapWithoutError(postIDs, func(id persist.DBID) string { return id.String() })
		return q.GetPostsByIds(ctx, ids)
	}
	return newPaginatorFromCursorWithF(cur, queryF)
}

func newPaginatorFromCursorWithF(cur string, queryF func([]persist.DBID) ([]db.Post, error)) (paginator feedPaginator, err error) {
	cursor := cursors.NewFeedPositionCursor()

	if err := cursor.Unpack(cur); err != nil {
		return paginator, err
	}

	paginator.QueryFunc = func(params feedPagingParams) ([]any, error) {
		posts, err := queryF(cursor.EntityIDs)
		postEdges := util.MapWithoutError(posts, func(p db.Post) any { return p })
		return postEdges, err
	}

	paginator.CursorFunc = func(node any) (int64, []persist.FeedEntityType, []persist.DBID, error) {
		_, id, err := feedCursor(node)
		if err != nil {
			return 0, cursor.EntityTypes, cursor.EntityIDs, err
		}
		pos, ok := cursor.PositionLookup[id]
		if !ok {
			panic(fmt.Sprintf("could not find position for id=%s", id))
		}
		return pos, cursor.EntityTypes, cursor.EntityIDs, err
	}

	return paginator, nil
}

func (api FeedAPI) TrendingFeed(ctx context.Context, before *string, after *string, first *int, last *int) ([]any, PageInfo, error) {
	// Validate
	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	var paginator feedPaginator
	var err error

	if before != nil {
		paginator, err = newPaginatorFromCursor(ctx, *before, api.queries)
		if err != nil {
			return nil, PageInfo{}, err
		}
	} else if after != nil {
		paginator, err = newPaginatorFromCursor(ctx, *after, api.queries)
		if err != nil {
			return nil, PageInfo{}, err
		}
	} else {
		var posts []db.Post

		cacheCalcFunc := func(ctx context.Context) ([]persist.FeedEntityType, []persist.DBID, error) {
			postScores, err := fetchFeedEntityScores(ctx, api.queries, "")
			if err != nil {
				return nil, nil, err
			}

			scores := util.MapWithoutError(util.MapValues(postScores), func(s db.GetFeedEntityScoresRow) db.FeedEntityScore { return s.FeedEntityScore })
			scored := api.scoreFeedEntities(ctx, 128, scores, func(e db.FeedEntityScore) float64 {
				return timeFactor(e.CreatedAt, time.Now()) * engagementFactor(int(e.Interactions))
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

		// Prime the cache
		cache := newFeedCache(api.cache, cacheCalcFunc)
		postTypes, postIDs, err := cache.Load(ctx)
		if err != nil {
			return nil, PageInfo{}, err
		}

		cursorable := cursorables.NewFeedPositionCursorer(func(node any) (int64, []persist.FeedEntityType, []persist.DBID, error) {
			return 0, postTypes, postIDs, nil
		})

		cursor, err := cursorable(nil)
		if err != nil {
			return nil, PageInfo{}, err
		}

		curString, err := cursor.Pack()
		if err != nil {
			return nil, PageInfo{}, err
		}

		if len(posts) == 0 {
			paginator, err = newPaginatorFromCursor(ctx, curString, api.queries)
		} else {
			paginator, err = newPaginatorFromCursorWithF(curString, func(postIDs []persist.DBID) ([]db.Post, error) { return posts, nil })
		}

		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	return paginator.paginate(before, after, first, last)
}

func (api FeedAPI) CuratedFeed(ctx context.Context, before, after *string, first, last *int) ([]any, PageInfo, error) {
	// Validate
	userID, _ := getAuthenticatedUserID(ctx)

	// Fallback to trending if no user
	if userID == "" {
		return api.TrendingFeed(ctx, before, after, first, last)
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	var err error
	var paginator feedPaginator

	if before != nil {
		paginator, err = newPaginatorFromCursor(ctx, *before, api.queries)
		if err != nil {
			return nil, PageInfo{}, err
		}
	} else if after != nil {
		paginator, err = newPaginatorFromCursor(ctx, *after, api.queries)
		if err != nil {
			return nil, PageInfo{}, err
		}
	} else {
		postScores, err := fetchFeedEntityScores(ctx, api.queries, userID)
		if err != nil {
			return nil, PageInfo{}, err
		}

		now := time.Now()
		scores := util.MapWithoutError(util.MapValues(postScores), func(s db.GetFeedEntityScoresRow) db.FeedEntityScore { return s.FeedEntityScore })
		engagementScores := make(map[persist.DBID]float64)
		personalizationScores := make(map[persist.DBID]float64)

		for _, e := range postScores {
			// Boost new events
			boost := 1.0
			if now.Sub(e.Post.CreatedAt) < 6*time.Hour {
				boost *= 2.0
			}
			timeF := timeFactor(e.Post.CreatedAt, now)
			engagementScores[e.Post.ID] = boost * timeF * (1 + engagementFactor(int(e.FeedEntityScore.Interactions)))
			personalizationScores[e.Post.ID] = boost * timeF * userpref.For(ctx).RelevanceTo(userID, e.FeedEntityScore)
		}

		// Rank by engagement first, then by personalization
		topNByEngagement := api.scoreFeedEntities(ctx, 128, scores, func(e db.FeedEntityScore) float64 { return engagementScores[e.ID] })
		topNByEngagement = api.scoreFeedEntities(ctx, 128, topNByEngagement, func(e db.FeedEntityScore) float64 { return personalizationScores[e.ID] })
		// Rank by personalization, then by engagement
		topNByPersonalization := api.scoreFeedEntities(ctx, 128, scores, func(e db.FeedEntityScore) float64 { return personalizationScores[e.ID] })
		topNByPersonalization = api.scoreFeedEntities(ctx, 128, topNByPersonalization, func(e db.FeedEntityScore) float64 { return engagementScores[e.ID] })

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
			return float64(engagementRank[e.ID]+personalizationRank[e.ID]) / 2.0
		})

		recommend.Shuffle(interleaved, 8)

		posts := make([]db.Post, len(interleaved))
		postIDs := make([]persist.DBID, len(interleaved))
		postTypes := make([]persist.FeedEntityType, len(interleaved))

		for i, e := range interleaved {
			idx := len(interleaved) - i - 1
			postIDs[idx] = e.ID
			posts[idx] = postScores[e.ID].Post
			postTypes[idx] = persist.FeedEntityType(e.FeedEntityType)
		}

		cursorable := cursorables.NewFeedPositionCursorer(func(node any) (int64, []persist.FeedEntityType, []persist.DBID, error) {
			return 0, postTypes, postIDs, nil
		})

		cursor, err := cursorable(nil)
		if err != nil {
			return nil, PageInfo{}, err
		}

		curString, err := cursor.Pack()
		if err != nil {
			return nil, PageInfo{}, err
		}

		paginator, err = newPaginatorFromCursorWithF(curString, func(postIDs []persist.DBID) ([]db.Post, error) { return posts, nil })
		if err != nil {
			return nil, PageInfo{}, err
		}
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

func feedCursor(i interface{}) (time.Time, persist.DBID, error) {
	switch row := i.(type) {
	case db.FeedEvent:
		return row.EventTime, row.ID, nil
	case db.Post:
		return row.CreatedAt, row.ID, nil
	}
	return time.Time{}, "", fmt.Errorf("interface{} is not a feed entity: %T", i)
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
		batchResults, batchErrs := d.FeedEventByFeedEventID.LoadAll(eventsFetch)
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
		batchResults, batchErrs := d.PostByPostID.LoadAll(postsFetch)
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

func (api FeedAPI) scoreFeedEntities(ctx context.Context, n int, trendData []db.FeedEntityScore, scoreF func(db.FeedEntityScore) float64) []db.FeedEntityScore {
	h := &heap{}

	var wg sync.WaitGroup

	scores := make([]entityScore, len(trendData))

	for i, event := range trendData {
		i := i
		event := event
		wg.Add(1)
		go func() {
			defer wg.Done()
			score := scoreF(event)
			scores[i] = entityScore{FeedEntityScore: event, score: score}
		}()
	}

	wg.Wait()

	for _, node := range scores {
		// Add first n items in the heap
		if h.Len() < n {
			heappkg.Push(h, node)
			continue
		}

		// If the score is greater than the smallest score in the heap, replace it
		if node.score > (*h)[0].(entityScore).score {
			heappkg.Pop(h)
			heappkg.Push(h, node)
		}
	}

	scoredEntities := make([]db.FeedEntityScore, h.Len())

	// Pop returns the smallest score first, so we reverse the order
	// such that the highest score is first
	i := h.Len() - 1
	for h.Len() > 0 {
		node := heappkg.Pop(h).(entityScore)
		scoredEntities[i] = node.FeedEntityScore
		i--
	}

	return scoredEntities
}

func timeFactor(t0, t1 time.Time) float64 {
	age := t1.Sub(t0).Minutes()
	return math.Pow(2, -(age / tHalf))
}

func engagementFactor(interactions int) float64 {
	return math.Log1p(float64(interactions))
}

type priorityNode interface {
	Score() float64
}

type entityScore struct {
	db.FeedEntityScore
	score float64
}

func (f entityScore) Score() float64 {
	return f.score
}

type heap []any

func (h heap) Len() int      { return len(h) }
func (h heap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *heap) Push(s any)   { *h = append(*h, s) }

func (h heap) Less(i, j int) bool {
	return h[i].(priorityNode).Score() < h[j].(priorityNode).Score()
}

func (h *heap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return item
}

type feedPagingParams struct {
	CurBeforePos int
	CurAfterPos  int
	EntityIDs    []persist.DBID
}

type feedPaginator struct {
	QueryFunc  func(params feedPagingParams) ([]any, error)
	CursorFunc func(node any) (pos int64, feedEntityType []persist.FeedEntityType, ids []persist.DBID, err error)
}

func (p *feedPaginator) paginate(before, after *string, first, last *int) ([]any, PageInfo, error) {
	args := feedPagingParams{
		CurBeforePos: defaultCursorBeforePosition,
		CurAfterPos:  defaultCursorAfterPosition,
	}

	beforeCur := cursors.NewFeedPositionCursor()
	afterCur := cursors.NewFeedPositionCursor()

	if before != nil {
		if err := beforeCur.Unpack(*before); err != nil {
			return nil, PageInfo{}, err
		}
		args.CurBeforePos = int(beforeCur.CurrentPosition)
		args.EntityIDs = beforeCur.EntityIDs
	}

	if after != nil {
		if err := afterCur.Unpack(*after); err != nil {
			return nil, PageInfo{}, err
		}
		args.CurAfterPos = int(afterCur.CurrentPosition)
		args.EntityIDs = afterCur.EntityIDs
	}

	results, err := p.QueryFunc(args)
	if err != nil {
		return nil, PageInfo{}, err
	}

	return pageFrom(results, nil, cursorables.NewFeedPositionCursorer(p.CursorFunc), before, after, first, last)
}

type feedCache struct {
	*redis.LazyCache
	CalcFunc func(context.Context) ([]persist.FeedEntityType, []persist.DBID, error)
}

func newFeedCache(cache *redis.Cache, f func(context.Context) ([]persist.FeedEntityType, []persist.DBID, error)) *feedCache {
	key := "trending:feedEvents:all"
	return &feedCache{
		LazyCache: &redis.LazyCache{
			Cache: cache,
			Key:   key,
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
