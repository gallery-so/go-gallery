package publicapi

import (
	"context"

	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/validate"
)

type NotificationsAPI struct {
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
}

func (api NotificationsAPI) GetViewerNotifications(ctx context.Context, before *persist.DBID, after *persist.DBID, first *int, last *int) ([]db.Notification, error) {
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

	if err := api.validator.Struct(validate.ConnectionPaginationParams{
		Before: before,
		After:  after,
		First:  first,
		Last:   last,
	}); err != nil {
		return nil, err
	}

	params := db.GetUserNotificationsBatchParams{OwnerID: userID}

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

	notifs, err := api.loaders.NotificationsByUserID.Load(params)

	return notifs, err
}

func (api NotificationsAPI) HasPage(ctx context.Context, cursor string, userId persist.DBID, byFirst bool) (bool, error) {
	notifID, err := model.Cursor.DecodeToDBID(&cursor)
	if err != nil {
		return false, err
	}

	return api.queries.UserFeedHasMoreNotifications(ctx, db.UserFeedHasMoreNotificationsParams{
		OwnerID:   userId,
		ID:        *notifID,
		FromFirst: byFirst,
	})

}

func (api NotificationsAPI) ClearUserNotifications(ctx context.Context, userId persist.DBID) ([]db.Notification, error) {
	return api.queries.ClearNotificationsForUser(ctx, userId)
}
