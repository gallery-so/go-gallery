package publicapi

import (
	"context"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
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

func (api FeedAPI) GetViewerFeed(ctx context.Context, before *persist.DBID, after *persist.DBID, first *int, last *int) (persist.DBID, []db.FeedEvent, error) {
	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return "", nil, err
	}

	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return "", nil, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return "", nil, err
	}

	params := db.GetUserFeedViewBatchParams{Follower: userID}

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

	events, err := api.loaders.FeedByUserID.Load(params)

	return userID, events, err
}

func (api FeedAPI) GlobalFeed(ctx context.Context, before *persist.DBID, after *persist.DBID, first *int, last *int) ([]db.FeedEvent, error) {
	// Validate
	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, err
	}

	params := db.GetGlobalFeedViewBatchParams{}

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
		return api.queries.UserFeedHasMoreEvents(ctx, db.UserFeedHasMoreEventsParams{
			Follower:  userId,
			ID:        *eventID,
			FromFirst: byFirst,
		})
	} else {
		return api.queries.GlobalFeedHasMoreEvents(ctx, db.GlobalFeedHasMoreEventsParams{
			ID:        *eventID,
			FromFirst: byFirst,
		})
	}
}
