package publicapi

import (
	"context"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/persist"
)

var defaultLastFeedToken persist.DBID
var defaultEventLimit = 24

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

func (api FeedAPI) ViewerFeed(ctx context.Context, opts ...FeedOption) ([]sqlc.FeedEvent, *persist.DBID, error) {
	settings := searchSettings{}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return nil, nil, err
	}

	opts = append(defaultFeedOptions(), opts...)
	opts = append(opts, WithViewer(userID))

	for _, opt := range opts {
		opt.Apply(&settings)
	}

	// Validate
	if err := validateFields(api.validator, validationMap{
		"viewer": {settings.viewer, "required"},
		"limit":  {settings.limit, "gte=0"},
	}); err != nil {
		return nil, nil, err
	}

	params := sqlc.GetUserFeedViewBatchParams{
		Follower: settings.viewer,
		ID:       settings.token,
		// fetch one extra to check if there are more results
		Limit: int32(settings.limit + 1),
	}

	events, err := api.loaders.FeedByUserId.Load(params)
	if err != nil {
		return nil, nil, err
	}

	if len(events) > settings.limit {
		token := events[settings.limit].ID
		events = events[:settings.limit]
		return events, &token, nil
	}

	return events, nil, nil
}

func (api FeedAPI) GlobalFeed(ctx context.Context, opts ...FeedOption) ([]sqlc.FeedEvent, *persist.DBID, error) {
	settings := searchSettings{}

	opts = append(defaultFeedOptions(), opts...)
	for _, opt := range opts {
		opt.Apply(&settings)
	}

	// Validate
	if err := validateFields(api.validator, validationMap{
		"limit": {settings.limit, "gte=0"},
	}); err != nil {
		return nil, nil, err
	}

	params := sqlc.GetGlobalFeedViewBatchParams{
		ID: settings.token,
		// fetch one extra to check if there are more results
		Limit: int32(settings.limit + 1),
	}

	events, err := api.loaders.GlobalFeed.Load(params)
	if err != nil {
		return nil, nil, err
	}

	if len(events) > settings.limit {
		token := events[settings.limit].ID
		events = events[:settings.limit]
		return events, &token, nil
	}

	return events, nil, nil
}

type searchSettings struct {
	viewer persist.DBID
	token  persist.DBID
	limit  int
}

type FeedOption interface {
	Apply(*searchSettings)
}

// WithPage fetches a subset of records.
func WithPage(page *model.Pagination) FeedOption {
	return withPage{page}
}

type withPage struct {
	page *model.Pagination
}

func (w withPage) Apply(s *searchSettings) {
	if w.page.Token != nil {
		s.token = *w.page.Token
	}
	if w.page.Limit != nil {
		s.limit = *w.page.Limit
	}
}

// WithViewer filters the feed to return a view for the provided userID
func WithViewer(userID persist.DBID) FeedOption {
	return withViewer{userID}
}

type withViewer struct {
	viewerID persist.DBID
}

func (w withViewer) Apply(s *searchSettings) {
	s.viewer = w.viewerID
}

func defaultFeedOptions() []FeedOption {
	return []FeedOption{
		withPage{
			page: &model.Pagination{
				Token: &defaultLastFeedToken,
				Limit: &defaultEventLimit,
			}},
	}
}
