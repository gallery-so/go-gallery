package publicapi

import (
	"context"
	"fmt"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
)

type FeedAPI struct {
	repos     *postgres.Repositories
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
}

func (api FeedAPI) GetEventById(ctx context.Context, feedEventID persist.DBID) (*db.FeedEvent, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
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

func (api FeedAPI) PaginatePersonalFeed(ctx context.Context, before *string, after *string, first *int, last *int) ([]db.FeedEvent, PageInfo, error) {
	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return nil, PageInfo{}, err
	}

	// Validate
	if err := validateFields(api.validator, validationMap{
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
	if err := validateFields(api.validator, validationMap{
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

func feedCursor(i interface{}) (time.Time, persist.DBID, error) {
	if row, ok := i.(db.FeedEvent); ok {
		return row.EventTime, row.ID, nil
	}
	return time.Time{}, "", fmt.Errorf("interface{} is not a feed event")
}
