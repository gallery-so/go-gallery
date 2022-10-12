package publicapi

import (
	"context"
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/persist"
)

type NotificationsAPI struct {
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
}

func (api NotificationsAPI) GetViewerNotifications(ctx context.Context, before, after *string, first *int, last *int) ([]db.Notification, PageInfo, error) {
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
		admires, err := api.loaders.NotificationsByUserID.Load(db.GetUserNotificationsBatchParams{
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

		results := make([]interface{}, len(admires))
		for i, admire := range admires {
			results[i] = admire
		}

		return results, nil
	}

	countFunc := func() (int, error) {
		total, err := api.queries.CountUserNotifications(ctx, userID)
		return int(total), err
	}

	cursorFunc := func(i interface{}) (time.Time, persist.DBID, error) {
		if admire, ok := i.(db.Admire); ok {
			return admire.CreatedAt, admire.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("interface{} is not an admire")
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	if err != nil {
		return nil, PageInfo{}, err
	}

	notifications := make([]db.Notification, len(results))
	for i, result := range results {
		if notification, ok := result.(db.Notification); ok {
			notifications[i] = notification
		} else {
			return nil, PageInfo{}, fmt.Errorf("interface{} is not a notification: %T", result)
		}
	}

	return notifications, pageInfo, err
}

func (api NotificationsAPI) GetByID(ctx context.Context, id persist.DBID) (db.Notification, error) {
	return api.loaders.NotificationByID.Load(id)
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

func (api NotificationsAPI) ClearUserNotifications(ctx context.Context) ([]db.Notification, error) {
	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	return api.queries.ClearNotificationsForUser(ctx, userID)
}
