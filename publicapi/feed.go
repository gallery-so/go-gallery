package publicapi

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/validate"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

type FeedAPI struct {
	repos     *postgres.Repositories
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
	cache     *redis.Cache
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

func (api FeedAPI) PaginatePersonalFeed(ctx context.Context, before *string, after *string, first *int, last *int) ([]persist.FeedEntity, PageInfo, error) {
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
		keys, err := api.loaders.PersonalFeedByUserID.Load(db.PaginatePersonalFeedByUserIDParams{
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

		results := make([]interface{}, len(keys))
		for i, key := range keys {
			results[i] = key
		}

		return results, nil
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: feedCursor,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	feedEvents := make([]persist.FeedEntity, len(results))
	for i, result := range results {
		feedEvents[i] = result.(persist.FeedEntity)
	}

	return feedEvents, pageInfo, err
}

func (api FeedAPI) PaginateUserFeed(ctx context.Context, userID persist.DBID, before *string, after *string,
	first *int, last *int) ([]persist.FeedEntity, PageInfo, error) {
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
		keys, err := api.loaders.UserFeedByUserID.Load(db.PaginateUserFeedByUserIDParams{
			OwnerID:       userID,
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

		results := make([]interface{}, len(keys))
		for i, key := range keys {
			results[i] = key
		}

		return results, nil
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: feedCursor,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	feedEvents := make([]persist.FeedEntity, len(results))
	for i, result := range results {
		feedEvents[i] = result.(persist.FeedEntity)
	}

	return feedEvents, pageInfo, err
}

func (api FeedAPI) PaginateGlobalFeed(ctx context.Context, before *string, after *string, first *int, last *int) ([]persist.FeedEntity, PageInfo, error) {
	// Validate
	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		keys, err := api.loaders.GlobalFeed.Load(db.PaginateGlobalFeedParams{
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

		results := make([]interface{}, len(keys))
		for i, key := range keys {
			results[i] = key
		}

		return results, nil
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: feedCursor,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	feedEvents := make([]persist.FeedEntity, len(results))
	for i, result := range results {
		feedEvents[i] = result.(persist.FeedEntity)
	}

	return feedEvents, pageInfo, err
}

func (api FeedAPI) PaginateTrendingFeed(ctx context.Context, before *string, after *string, first *int, last *int) ([]persist.FeedEntity, PageInfo, error) {
	// Validate
	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	var (
		err         error
		trendingIDs []persist.DBID
		paginator   = positionPaginator{}
		lookup      = make(map[persist.DBID]int)
		// lambda determines how quickly the score decreases over time. A larger value (i.e. a smaller denominator) has a faster decay rate.
		lambda     = float64(1) / 100000
		reportSize = 100
	)

	// If a cursor is provided, we can skip querying the cache
	if before != nil {
		if _, trendingIDs, err = paginator.decodeCursor(*before); err != nil {
			return nil, PageInfo{}, err
		}
	} else if after != nil {
		if _, trendingIDs, err = paginator.decodeCursor(*after); err != nil {
			return nil, PageInfo{}, err
		}
	} else {
		calcFunc := func(ctx context.Context) ([]persist.DBID, error) {
			trendData, err := api.queries.GetTrendingFeedEventIDs(ctx, time.Now().Add(-time.Duration(72*time.Hour)))
			if err != nil {
				return nil, err
			}

			// Compute a new score for each event by weighting the current score by its relative age
			scores := make(map[persist.DBID]float64)
			now := time.Now()
			for _, event := range trendData {
				eventAge := now.Sub(event.CreatedAt).Seconds()
				score := float64(event.Count) * math.Pow(math.E, (-lambda*eventAge))
				// Invert the score so that events are in descending order (in terms of absolute value) below
				scores[event.ID] = -score
			}

			sort.Slice(trendData, func(i, j int) bool {
				return scores[trendData[i].ID] < scores[trendData[j].ID]
			})

			trendingIDs := make([]persist.DBID, 0)

			for i := 0; i < len(trendData) && i < reportSize; i++ {
				trendingIDs = append(trendingIDs, trendData[i].ID)
			}

			return trendingIDs, nil
		}

		l := newDBIDCache(api.cache, "trending:feedEvents", time.Minute*10, calcFunc)

		trendingIDs, err = l.Load(ctx)
		if err != nil {
			return nil, PageInfo{}, err
		}
	}

	asStr := make([]string, len(trendingIDs))
	for i, id := range trendingIDs {
		// Postgres uses 1-based indexing
		lookup[id] = i + 1
		asStr[i] = id.String()
	}

	queryFunc := func(params positionPagingParams) ([]any, error) {
		keys, err := api.queries.PaginateTrendingFeed(ctx, db.PaginateTrendingFeedParams{
			FeedEventIds:  asStr,
			CurBeforePos:  params.CursorBeforePos,
			CurAfterPos:   params.CursorAfterPos,
			PagingForward: params.PagingForward,
			Limit:         params.Limit,
		})

		results, _ := util.Map(keys, func(k persist.FeedEntity) (any, error) {
			return k, nil
		})

		return results, err
	}

	cursorFunc := func(node any) (int, []persist.DBID, error) {
		_, id, err := feedCursor(node)
		return lookup[id], trendingIDs, err
	}

	paginator.QueryFunc = queryFunc
	paginator.CursorFunc = cursorFunc
	results, pageInfo, err := paginator.paginate(before, after, first, last)

	feedEvents, _ := util.Map(results, func(r any) (persist.FeedEntity, error) {
		return r.(persist.FeedEntity), nil
	})

	return feedEvents, pageInfo, err
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
	if row, ok := i.(db.FeedEvent); ok {
		return row.EventTime, row.ID, nil
	}
	return time.Time{}, "", fmt.Errorf("interface{} is not a feed event")
}
