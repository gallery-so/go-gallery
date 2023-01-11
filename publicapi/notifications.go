package publicapi

import (
	"context"
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/validate"
)

type NotificationsAPI struct {
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
}

func (api NotificationsAPI) GetViewerNotifications(ctx context.Context, before, after *string, first *int, last *int) ([]db.Notification, PageInfo, int, error) {
	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return nil, PageInfo{}, 0, err
	}

	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, PageInfo{}, 0, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, 0, err
	}

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {
		notifs, err := api.loaders.NotificationsByUserID.Load(db.GetUserNotificationsBatchParams{
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

		results := make([]interface{}, len(notifs))
		for i, notif := range notifs {
			results[i] = notif
		}

		return results, nil
	}

	countFunc := func() (int, error) {
		total, err := api.queries.CountUserNotifications(ctx, userID)
		return int(total), err
	}

	cursorFunc := func(i interface{}) (time.Time, persist.DBID, error) {
		if admire, ok := i.(db.Notification); ok {
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
		return nil, PageInfo{}, 0, err
	}

	notifications := make([]db.Notification, len(results))
	for i, result := range results {
		if notification, ok := result.(db.Notification); ok {
			notifications[i] = notification
		} else {
			return nil, PageInfo{}, 0, fmt.Errorf("interface{} is not a notification: %T", result)
		}
	}

	count, err := api.queries.CountUserUnseenNotifications(ctx, userID)
	if err != nil {
		return nil, PageInfo{}, 0, err
	}

	return notifications, pageInfo, int(count), err
}

func (api NotificationsAPI) GetByID(ctx context.Context, id persist.DBID) (db.Notification, error) {
	return api.loaders.NotificationByID.Load(id)
}

func (api NotificationsAPI) ClearUserNotifications(ctx context.Context) ([]db.Notification, error) {
	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	return api.queries.ClearNotificationsForUser(ctx, userID)
}
