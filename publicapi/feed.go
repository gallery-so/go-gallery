package publicapi

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
)

type FeedAPI struct {
	repos     *persist.Repositories
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
}

func (api FeedAPI) GetEventById(ctx context.Context, eventID persist.DBID) (*db.FeedEvent, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"eventID": {eventID, "required"},
	}); err != nil {
		return nil, err
	}

	event, err := api.loaders.EventByEventID.Load(eventID)
	if err != nil {
		return nil, err
	}

	return &event, nil
}

func (api FeedAPI) PaginatePersonalFeedByEventID(ctx context.Context, before *string, after *string, first *int, last *int) ([]db.FeedEvent, PageInfo, error) {
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
		keys, err := api.loaders.PersonalFeedByUserID.Load(db.PaginatePersonalFeedByFeedEventIDParams{
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

	countFunc := func() (int, error) {
		total, err := api.queries.CountPersonalFeedEventsByFollowerID(ctx, userID)
		return int(total), err
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CountFunc:  countFunc,
		CursorFunc: feedCursor,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	feedEvents := make([]db.FeedEvent, len(results))
	for i, result := range results {
		feedEvents[i] = result.(db.FeedEvent)
	}

	return feedEvents, pageInfo, err
}

func (api FeedAPI) PaginateUserFeedByEventID(ctx context.Context, userID persist.DBID, before *string, after *string,
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
		keys, err := api.loaders.UserFeedByUserID.Load(db.PaginateUserFeedByFeedEventIDParams{
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

	countFunc := func() (int, error) {
		total, err := api.queries.CountFeedEventsByUserID(ctx, userID)
		return int(total), err
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CountFunc:  countFunc,
		CursorFunc: feedCursor,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	feedEvents := make([]db.FeedEvent, len(results))
	for i, result := range results {
		feedEvents[i] = result.(db.FeedEvent)
	}

	return feedEvents, pageInfo, err
}

func (api FeedAPI) PaginateGlobalFeedByEventID(ctx context.Context, before *string, after *string, first *int, last *int) ([]db.FeedEvent, PageInfo, error) {
	// Validate
	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		keys, err := api.loaders.GlobalFeed.Load(db.PaginateGlobalFeedByFeedEventIDParams{
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

	countFunc := func() (int, error) {
		total, err := api.queries.CountGlobalFeedEvents(ctx)
		return int(total), err
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CountFunc:  countFunc,
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
		return row.CreatedAt, row.ID, nil
	}
	return time.Time{}, "", fmt.Errorf("interface{} is not a feed event")
}
