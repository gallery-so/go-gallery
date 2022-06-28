package publicapi

import (
	"context"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/validate"
)

var defaultTokenParam = "<notset>"

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

func (api FeedAPI) GetFeedByUserID(ctx context.Context, userID persist.DBID, before *persist.DBID, after *persist.DBID, first *int, last *int) ([]sqlc.FeedEvent, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID": {userID, "required"},
		"first":  {first, "omitempty,gte=0"},
		"last":   {last, "omitempty,gte=0"},
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

	params := sqlc.GetUserFeedViewBatchParams{
		Follower:  userID,
		CurBefore: defaultTokenParam,
		CurAfter:  defaultTokenParam,
	}

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

	return api.loaders.FeedByUserId.Load(params)
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
