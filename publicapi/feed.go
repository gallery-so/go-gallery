package publicapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		"userId": {userId, "required"},
		"action": {action, "required"},
	})

	if err != nil {
		return err
	}

	err = api.queries.BlockUserFromFeed(ctx, db.BlockUserFromFeedParams{
		ID:     persist.GenerateID(),
		UserID: userId,
		Action: action,
	})

	return err
}

func (api FeedAPI) GetFeedEventById(ctx context.Context, feedEventID persist.DBID) (*db.FeedEvent, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"feedEventID": {feedEventID, "required"},
	}); err != nil {
		return nil, err
	}

	event, err := api.loaders.FeedEventByFeedEventID.Load(feedEventID)
	if err != nil {
		return nil, err
	}

	return &event, nil
}

func (api FeedAPI) GetRawEventById(ctx context.Context, eventID persist.DBID) (*db.Event, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"eventID": {eventID, "required"},
	}); err != nil {
		return nil, err
	}

	event, err := api.queries.GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}

	return &event, nil
}

func (api FeedAPI) PaginatePersonalFeed(ctx context.Context, before *string, after *string, first *int, last *int) ([]db.FeedEvent, PageInfo, error) {
	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return nil, PageInfo{}, err
	}

	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": {userID, "required"},
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

	feedEvents := make([]db.FeedEvent, len(results))
	for i, result := range results {
		feedEvents[i] = result.(db.FeedEvent)
	}

	return feedEvents, pageInfo, err
}

func (api FeedAPI) PaginateUserFeed(ctx context.Context, userID persist.DBID, before *string, after *string,
	first *int, last *int) ([]db.FeedEvent, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": {userID, "required"},
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

	feedEvents := make([]db.FeedEvent, len(results))
	for i, result := range results {
		feedEvents[i] = result.(db.FeedEvent)
	}

	return feedEvents, pageInfo, err
}

func (api FeedAPI) PaginateGlobalFeed(ctx context.Context, before *string, after *string, first *int, last *int) ([]db.FeedEvent, PageInfo, error) {
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

	feedEvents := make([]db.FeedEvent, len(results))
	for i, result := range results {
		feedEvents[i] = result.(db.FeedEvent)
	}

	return feedEvents, pageInfo, err
}

func (api FeedAPI) PaginateTrendingFeed(ctx context.Context, before *string, after *string, first *int, last *int) ([]db.FeedEvent, PageInfo, error) {
	// Validate
	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	var (
		err         error
		trendingIDs []persist.DBID
		paginator   = positionPaginator{}
		lookup      = make(map[persist.DBID]int)
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
			return api.queries.GetTrendingFeedEventIDs(ctx, db.GetTrendingFeedEventIDsParams{
				WindowEnd: time.Now().Add(-time.Duration(72 * time.Hour)),
				Limit:     100,
			})
		}

		t := trender{
			Cache:    api.cache,
			Key:      "trending.feedEvents",
			TTL:      time.Hour,
			CalcFunc: calcFunc,
		}
		if err = t.load(ctx, &trendingIDs); err != nil {
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
		if err != nil {
			return nil, err
		}

		results := make([]interface{}, len(keys))
		for i, key := range keys {
			results[i] = key
		}

		return results, nil
	}

	cursorFunc := func(node any) (int, []persist.DBID, error) {
		_, id, err := feedCursor(node)
		return lookup[id], trendingIDs, err
	}

	paginator.QueryFunc = queryFunc
	paginator.CursorFunc = cursorFunc
	results, pageInfo, err := paginator.paginate(before, after, first, last)

	feedEvents := make([]db.FeedEvent, len(results))
	for i, result := range results {
		feedEvents[i] = result.(db.FeedEvent)
	}

	return feedEvents, pageInfo, err
}

func (api FeedAPI) TrendingUsers(ctx context.Context, report model.Window) ([]db.User, error) {
	calcFunc := func(ctx context.Context) ([]persist.DBID, error) {
		return api.queries.GetTrendingUserIDs(ctx, db.GetTrendingUserIDsParams{
			WindowEnd: time.Now().Add(-time.Duration(report.Duration)),
			Size:      24,
		})
	}

	// Reports that calculating trending users greater than a week or more
	// are calculated once every 24 hours rather than once an hour.
	ttl := time.Hour
	if report.Duration > 7*24*time.Hour {
		ttl *= 24
	}

	t := trender{
		Cache:    api.cache,
		Key:      "trending.users." + report.Name,
		TTL:      ttl,
		CalcFunc: calcFunc,
	}

	var trendingIDs []persist.DBID

	err := t.load(ctx, &trendingIDs)
	if err != nil {
		return nil, err
	}

	asStr := make([]string, len(trendingIDs))
	for i, id := range trendingIDs {
		asStr[i] = id.String()
	}

	return api.queries.GetTrendingUsersByIDs(ctx, asStr)
}

func feedCursor(i interface{}) (time.Time, persist.DBID, error) {
	if row, ok := i.(db.FeedEvent); ok {
		return row.EventTime, row.ID, nil
	}
	return time.Time{}, "", fmt.Errorf("interface{} is not a feed event")
}

type trender struct {
	// CalcFunc computes the results of a trend and returns record in sorted order
	CalcFunc func(context.Context) ([]persist.DBID, error)
	Cache    *redis.Cache  // Cache to store pre-computed trends
	Key      string        // Key used to store and reference saved results
	TTL      time.Duration // The length of time before a cached result is considered to be stale
}

// load implements a lazy filled cache where only requested data is stored in the cache.
// It is possible for load to return stale data, however the staleness of data can be
// limited by configuring a shorter TTL. The tradeoff being that a shorter TTL results in more
// cache misses and more frequent trips to check the cache, compute the trend, and re-populate the cache.
func (t *trender) load(ctx context.Context, into any) error {
	byt, err := t.Cache.Get(ctx, t.Key)
	var notFoundErr redis.ErrKeyNotFound

	if err != nil && !errors.As(err, &notFoundErr) {
		return err
	}

	if errors.As(err, &notFoundErr) {
		trend, err := t.CalcFunc(ctx)
		if err != nil {
			return err
		}

		byt, err = json.Marshal(trend)
		if err != nil {
			return err
		}

		err = t.Cache.Set(ctx, t.Key, byt, t.TTL)
	}

	return json.Unmarshal(byt, &into)
}
