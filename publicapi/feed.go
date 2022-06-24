package publicapi

import (
	"context"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/publicapi/option"
	"github.com/mikeydub/go-gallery/service/persist"
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

func (api FeedAPI) ViewerFeed(ctx context.Context, opts ...option.FeedOption) ([]sqlc.FeedEvent, *persist.DBID, error) {
	settings := option.FeedSearchSettings{}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return nil, nil, err
	}

	opts = append(option.DefaultFeedOptions(), opts...)

	for _, opt := range opts {
		opt.Apply(&settings)
	}

	// Validate
	if err := validateFields(api.validator, validationMap{
		"limit": {settings.Limit, "gte=0"},
	}); err != nil {
		return nil, nil, err
	}

	params := sqlc.GetUserFeedViewBatchParams{
		Follower: userID,
		ID:       settings.Token,
		// fetch one extra to check if there are more results
		Limit: int32(settings.Limit + 1),
	}

	events, err := api.loaders.FeedByUserId.Load(params)
	if err != nil {
		return nil, nil, err
	}

	if len(events) > settings.Limit {
		token := events[settings.Limit].ID
		events = events[:settings.Limit]
		return events, &token, nil
	}

	return events, nil, nil
}

func (api FeedAPI) GlobalFeed(ctx context.Context, opts ...option.FeedOption) ([]sqlc.FeedEvent, *persist.DBID, error) {
	settings := option.FeedSearchSettings{}

	opts = append(option.DefaultFeedOptions(), opts...)
	for _, opt := range opts {
		opt.Apply(&settings)
	}

	// Validate
	if err := validateFields(api.validator, validationMap{
		"limit": {settings.Limit, "gte=0"},
	}); err != nil {
		return nil, nil, err
	}

	params := sqlc.GetGlobalFeedViewBatchParams{
		ID: settings.Token,
		// fetch one extra to check if there are more results
		Limit: int32(settings.Limit + 1),
	}

	events, err := api.loaders.GlobalFeed.Load(params)
	if err != nil {
		return nil, nil, err
	}

	if len(events) > settings.Limit {
		token := events[settings.Limit].ID
		events = events[:settings.Limit]
		return events, &token, nil
	}

	return events, nil, nil
}
