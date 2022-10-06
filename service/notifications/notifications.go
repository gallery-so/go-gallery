package notifications

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

const window = 10 * time.Minute

const NotificationHandlerContextKey = "notification.notificationHandlers"

type NotificationHandlers struct {
	Notifications            *notifcationDispatcher
	UserNewNotifications     map[persist.DBID]chan db.Notification
	UserUpdatedNotifications map[persist.DBID]chan db.Notification
}

// Register specific notification handlers
func AddTo(ctx *gin.Context, queries *db.Queries) {
	notifDispatcher := notifcationDispatcher{handlers: map[persist.Action]notificationHandler{}}
	new := map[persist.DBID]chan db.Notification{}
	updated := map[persist.DBID]chan db.Notification{}
	def := defaultNotificationHandler{queries: queries, new: new}
	group := groupedNotificationHandler{queries: queries, new: new, updated: updated}

	notifDispatcher.AddHandler(persist.ActionUserFollowedUsers, group)
	notifDispatcher.AddHandler(persist.ActionUserFollowedUserBack, group)
	notifDispatcher.AddHandler(persist.ActionAdmiredFeedEvent, group)
	notifDispatcher.AddHandler(persist.ActionCommentedOnFeedEvent, def)
	notifDispatcher.AddHandler(persist.ActionViewedGallery, group)

	notificationHandlers := &NotificationHandlers{Notifications: &notifDispatcher}
	ctx.Set(NotificationHandlerContextKey, notificationHandlers)
}

func DispatchNotificationToUser(ctx context.Context, notif db.Notification) error {
	gc := util.GinContextFromContext(ctx)
	return For(gc).Notifications.Dispatch(ctx, notif)
}

func For(ctx context.Context) *NotificationHandlers {
	gc := util.GinContextFromContext(ctx)
	return gc.Value(NotificationHandlerContextKey).(*NotificationHandlers)
}

func (n *NotificationHandlers) GetNewNotificationsForUser(userID persist.DBID) chan db.Notification {
	if sub, ok := n.UserNewNotifications[userID]; ok {
		return sub
	}
	sub := make(chan db.Notification)
	n.UserNewNotifications[userID] = sub
	return sub
}

func (n *NotificationHandlers) GetUpdatedNotificationsForUser(userID persist.DBID) chan db.Notification {
	if sub, ok := n.UserUpdatedNotifications[userID]; ok {
		return sub
	}
	sub := make(chan db.Notification)
	n.UserUpdatedNotifications[userID] = sub
	return sub
}

func (n *NotificationHandlers) UnscubscribeNewNotificationsForUser(userID persist.DBID) {
	n.UserNewNotifications[userID] = nil
}

func (n *NotificationHandlers) UnsubscribeUpdatedNotificationsForUser(userID persist.DBID) {
	n.UserUpdatedNotifications[userID] = nil
}

type notificationHandler interface {
	Handle(context.Context, db.Notification) error
}

type notifcationDispatcher struct {
	handlers map[persist.Action]notificationHandler
}

func (d *notifcationDispatcher) AddHandler(action persist.Action, handler notificationHandler) {
	d.handlers[action] = handler
}

func (d *notifcationDispatcher) Dispatch(ctx context.Context, notif db.Notification) error {
	if handler, ok := d.handlers[notif.Action]; ok {
		return handler.Handle(ctx, notif)
	}
	logger.For(ctx).Warnf("no handler registered for action: %s", notif.Action)
	return nil
}

type defaultNotificationHandler struct {
	queries *coredb.Queries
	new     map[persist.DBID]chan db.Notification
}

func (h defaultNotificationHandler) Handle(ctx context.Context, notif db.Notification) error {
	newNotif, err := h.queries.CreateNotification(ctx, db.CreateNotificationParams{
		ID:      notif.ID,
		OwnerID: notif.OwnerID,
		ActorID: notif.ActorID,
		Action:  notif.Action,
		Data:    notif.Data,
	})
	if err != nil {
		return err
	}

	if sub, ok := h.new[newNotif.OwnerID]; ok {
		select {
		case sub <- newNotif:
		default:
			logger.For(ctx).Warnf("notification channel not open for user: %d", notif.OwnerID)
			h.new[newNotif.OwnerID] = nil
		}
	}
	return nil
}

type groupedNotificationHandler struct {
	queries *coredb.Queries
	new     map[persist.DBID]chan db.Notification
	updated map[persist.DBID]chan db.Notification
}

func (h groupedNotificationHandler) Handle(ctx context.Context, notif db.Notification) error {
	notifID := notif.ID
	var createdAtTime time.Time
	if notifID != "" {
		curNotif, _ := h.queries.GetNotificationByID(ctx, notif.ID)
		if curNotif.ID != "" {
			notifID = curNotif.ID
			createdAtTime = curNotif.CreatedAt
		}
	}
	if notifID != "" && time.Since(createdAtTime) < window {
		err := h.queries.UpdateNotification(ctx, db.UpdateNotificationParams{
			ID:     notifID,
			Data:   notif.Data,
			Amount: notif.Amount,
		})
		if err != nil {
			return err
		}
		curNotif, err := h.queries.GetNotificationByID(ctx, notif.ID)
		if err != nil {
			return err
		}
		if sub, ok := h.updated[curNotif.OwnerID]; ok {
			select {
			case sub <- curNotif:
			default:
				logger.For(ctx).Warnf("notification update channel not open for user: %d", notif.OwnerID)
				h.updated[curNotif.OwnerID] = nil
			}
		}
	} else {
		newNotif, err := h.queries.CreateNotification(ctx, db.CreateNotificationParams{
			ID:      notif.ID,
			OwnerID: notif.OwnerID,
			ActorID: notif.ActorID,
			Action:  notif.Action,
			Data:    notif.Data,
		})
		if err != nil {
			return err
		}
		if sub, ok := h.new[newNotif.OwnerID]; ok {
			select {
			case sub <- newNotif:
			default:
				logger.For(ctx).Warnf("notification create channel not open for user: %d", notif.OwnerID)
				h.new[newNotif.OwnerID] = nil
			}
		}
	}

	return nil
}
