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

func (api FeedAPI) TrendingUserIDs(ctx context.Context, report model.Window) (users []persist.DBID, err error) {
	byt, err := api.cache.Get(ctx, "trending.users."+report.Name)
	var notFoundErr redis.ErrKeyNotFound
	switch {
	case errors.As(err, &notFoundErr):
		users, err := api.queries.GetTrendingUsers(ctx, db.GetTrendingUsersParams{
			WindowEnd: time.Now().Add(-time.Duration(report.Duration)),
			Size:      20,
		})
		if err != nil {
			return nil, err
		}

		byt, err := json.Marshal(users)
		if err != nil {
			return nil, err
		}

		ttl := time.Hour
		if report.Duration > 7*24*time.Hour {
			ttl *= 24
		}

		err = api.cache.Set(ctx, "trending.users."+report.Name, byt, ttl)
		if err != nil {
			return nil, err
		}

		return users, nil
	case err != nil:
		return nil, err
	default:
		err := json.Unmarshal(byt, &users)
		if err != nil {
			return nil, err
		}
		return users, nil
	}
}

func feedCursor(i interface{}) (time.Time, persist.DBID, error) {
	if row, ok := i.(db.FeedEvent); ok {
		return row.EventTime, row.ID, nil
	}
	return time.Time{}, "", fmt.Errorf("interface{} is not a feed event")
}
