package publicapi

import (
	"context"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/validate"
)

type FeedAPI struct {
	repos     *persist.Repositories
	queries   *sqlc.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
}

func (api FeedAPI) GetEventById(ctx context.Context, eventID persist.DBID) (*sqlc.FeedEvent, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"eventID": {eventID, "required"},
	}); err != nil {
		return nil, err
	}

	event, err := api.loaders.EventByEventId.Load(eventID)
	if err != nil {
		return nil, err
	}

	return &event, nil
}

func (api FeedAPI) GetViewerFeed(ctx context.Context, before *persist.DBID, after *persist.DBID, first *int, last *int) (persist.DBID, []sqlc.FeedEvent, error) {
	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return "", nil, err
	}

	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID": {userID, "required"},
		"first":  {first, "omitempty,gte=0"},
		"last":   {last, "omitempty,gte=0"},
	}); err != nil {
		return "", nil, err
	}

	if err := api.validator.Struct(validate.ConnectionPaginationParams{
		Before: before,
		After:  after,
		First:  first,
		Last:   last,
	}); err != nil {
		return "", nil, err
	}

	params := sqlc.GetUserFeedViewBatchParams{Follower: userID}

	if first != nil {
		params.FromFirst = true
		params.Limit = int32(*first)
	}

	if last != nil {
		params.FromFirst = false
		params.Limit = int32(*last)
	}

	if before != nil {
		params.CurBefore = string(*before)
	}

	if after != nil {
		params.CurAfter = string(*after)
	}

	events, err := api.loaders.FeedByUserId.Load(params)

	return userID, events, err
}

func (api FeedAPI) GlobalFeed(ctx context.Context, before *persist.DBID, after *persist.DBID, first *int, last *int) ([]sqlc.FeedEvent, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"first": {first, "omitempty,gte=0"},
		"last":  {last, "omitempty,gte=0"},
	}); err != nil {
		return nil, err
	}

	if err := api.validator.Struct(validate.ConnectionPaginationParams{
		Before: before,
		After:  after,
		First:  first,
		Last:   last,
	}); err != nil {
		return nil, err
	}

	params := sqlc.GetGlobalFeedViewBatchParams{}

	if first != nil {
		params.FromFirst = true
		params.Limit = int32(*first)
	}

	if last != nil {
		params.FromFirst = false
		params.Limit = int32(*last)
	}

	if before != nil {
		params.CurBefore = string(*before)
	}

	if after != nil {
		params.CurAfter = string(*after)
	}

	return api.loaders.GlobalFeed.Load(params)
}

func (api FeedAPI) HasPage(ctx context.Context, cursor string, userId persist.DBID, byFirst bool) (bool, error) {
	eventID, err := model.Cursor.DecodeToDBID(&cursor)
	if err != nil {
		return false, err
	}

	if userId != "" {
		return api.queries.UserFeedHasMoreEvents(ctx, sqlc.UserFeedHasMoreEventsParams{
			Follower:  userId,
			ID:        *eventID,
			FromFirst: byFirst,
		})
	} else {
		return api.queries.GlobalFeedHasMoreEvents(ctx, sqlc.GlobalFeedHasMoreEventsParams{
			ID:        *eventID,
			FromFirst: byFirst,
		})
	}
}
