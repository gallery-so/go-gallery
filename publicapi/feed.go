package publicapi

import (
	"context"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
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

func (api FeedAPI) ViewerFeed(ctx context.Context, before *persist.DBID, after *persist.DBID, first *int, last *int) ([]sqlc.FeedEvent, error) {
	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return nil, err
	}

	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID": {userID, "required"},
		"first":  {first, "omitempty,gte=0"},
		"last":   {last, "omitempty,gte=0"},
	}); err != nil {
		return nil, err
	}

	params := sqlc.GetUserFeedViewBatchParams{
		Follower:  userID,
		CurBefore: defaultTokenParam,
		CurAfter:  defaultTokenParam,
		UnsetFlag: defaultTokenParam,
	}

	if before != nil {
		params.CurBefore = string(*before)
	}

	if after != nil {
		params.CurAfter = string(*after)
	}

	events, err := api.loaders.FeedByUserId.Load(params)

	if err != nil {
		return nil, err
	}

	return events, nil
}

func (api FeedAPI) GlobalFeed(ctx context.Context, before *persist.DBID, after *persist.DBID, first *int, last *int) ([]sqlc.FeedEvent, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"first": {first, "omitempty,gte=0"},
		"last":  {last, "omitempty,gte=0"},
	}); err != nil {
		return nil, err
	}

	params := sqlc.GetGlobalFeedViewBatchParams{
		CurBefore: defaultTokenParam,
		CurAfter:  defaultTokenParam,
		UnsetFlag: defaultTokenParam,
	}

	if before != nil {
		params.CurBefore = string(*before)
	}

	if after != nil {
		params.CurAfter = string(*after)
	}

	events, err := api.loaders.GlobalFeed.Load(params)

	if err != nil {
		return nil, err
	}

	return events, nil
}
