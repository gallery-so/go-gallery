package notifications

import (
	"context"
	"encoding/json"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

const window = 10 * time.Minute
const notificationTimeout = 10 * time.Second
const NotificationHandlerContextKey = "notification.notificationHandlers"

type NotificationHandlers struct {
	Notifications            *notifcationDispatcher
	UserNewNotifications     map[persist.DBID]chan db.Notification
	UserUpdatedNotifications map[persist.DBID]chan db.Notification
	pubSub                   *pubsub.Client
}

// Register specific notification handlers
func AddTo(ctx *gin.Context, queries *db.Queries, pub *pubsub.Client) {
	notifDispatcher := notifcationDispatcher{handlers: map[persist.Action]notificationHandler{}}

	def := defaultNotificationHandler{queries: queries, pubSub: pub}
	group := groupedNotificationHandler{queries: queries, pubSub: pub}

	notifDispatcher.AddHandler(persist.ActionUserFollowedUsers, group)
	notifDispatcher.AddHandler(persist.ActionUserFollowedUserBack, group)
	notifDispatcher.AddHandler(persist.ActionAdmiredFeedEvent, group)
	notifDispatcher.AddHandler(persist.ActionCommentedOnFeedEvent, def)
	notifDispatcher.AddHandler(persist.ActionViewedGallery, group)

	new := map[persist.DBID]chan db.Notification{}
	updated := map[persist.DBID]chan db.Notification{}

	notificationHandlers := &NotificationHandlers{Notifications: &notifDispatcher, UserNewNotifications: new, UserUpdatedNotifications: updated, pubSub: pub}
	ctx.Set(NotificationHandlerContextKey, notificationHandlers)
	go notificationHandlers.receiveNewNotificationsFromPubSub()
	go notificationHandlers.receiveUpdatedNotificationsFromPubSub()
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
	pubSub  *pubsub.Client
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

	marshalled, err := json.Marshal(newNotif)
	if err != nil {
		return err
	}
	t := h.pubSub.Topic(viper.GetString("PUBSUB_TOPIC_NEW_NOTIFICATIONS"))
	result := t.Publish(ctx, &pubsub.Message{
		Data: marshalled,
	})

	_, err = result.Get(ctx)
	if err != nil {
		return err
	}
	return nil
}

type groupedNotificationHandler struct {
	queries *coredb.Queries
	pubSub  *pubsub.Client
}

func (h groupedNotificationHandler) Handle(ctx context.Context, notif db.Notification) error {
	notifID := notif.ID
	var curNotif db.Notification
	if notifID != "" {
		curNotif, _ = h.queries.GetNotificationByID(ctx, notif.ID)
		if curNotif.ID != "" {
			notifID = curNotif.ID
		}
	}
	if notifID != "" && time.Since(curNotif.CreatedAt) < window {
		err := h.queries.UpdateNotification(ctx, db.UpdateNotificationParams{
			ID:     notifID,
			Data:   createNewData(curNotif.Data, notif.Data),
			Amount: notif.Amount,
		})
		if err != nil {
			return err
		}
		curNotif, err := h.queries.GetNotificationByID(ctx, notif.ID)
		if err != nil {
			return err
		}
		marshalled, err := json.Marshal(curNotif)
		if err != nil {
			return err
		}
		t := h.pubSub.Topic(viper.GetString("PUBSUB_TOPIC_UPDATED_NOTIFICATIONS"))
		result := t.Publish(ctx, &pubsub.Message{
			Data: marshalled,
		})
		_, err = result.Get(ctx)
		if err != nil {
			return err
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
		marshalled, err := json.Marshal(newNotif)
		if err != nil {
			return err
		}

		t := h.pubSub.Topic(viper.GetString("PUBSUB_TOPIC_NEW_NOTIFICATIONS"))
		result := t.Publish(ctx, &pubsub.Message{
			Data: marshalled,
		})
		_, err = result.Get(ctx)
		if err != nil {
			return err
		}

	}

	return nil
}

func (n *NotificationHandlers) receiveNewNotificationsFromPubSub() {
	sub := n.pubSub.Subscription(viper.GetString("PUBSUB_SUB_NEW_NOTIFICATIONS"))

	err := sub.Receive(context.Background(), func(ctx context.Context, msg *pubsub.Message) {
		defer msg.Ack()
		notif := db.Notification{}
		err := json.Unmarshal(msg.Data, &notif)
		if err != nil {
			logger.For(ctx).Warnf("failed to unmarshal pubsub message: %s", err)
			return
		}
		if sub, ok := n.UserNewNotifications[notif.OwnerID]; ok {
			select {
			case sub <- notif:
			case <-time.After(notificationTimeout):
				logger.For(ctx).Warnf("notification create channel not open for user: %s", notif.OwnerID)
				n.UserNewNotifications[notif.OwnerID] = nil
			}
		}
	})
	if err != nil {
		logger.For(nil).Errorf("error receiving new notifications from pubsub: %s", err)
		panic(err)
	}
}

func (n *NotificationHandlers) receiveUpdatedNotificationsFromPubSub() {
	sub := n.pubSub.Subscription(viper.GetString("PUBSUB_UPDATED_NOTIFICATIONS_SUBSCRIPTION"))

	err := sub.Receive(context.Background(), func(ctx context.Context, msg *pubsub.Message) {
		defer msg.Ack()
		notif := db.Notification{}
		err := json.Unmarshal(msg.Data, &notif)
		if err != nil {
			logger.For(ctx).Warnf("failed to unmarshal pubsub message: %s", err)
			return
		}
		if sub, ok := n.UserUpdatedNotifications[notif.OwnerID]; ok {
			select {
			case sub <- notif:
			case <-time.After(notificationTimeout):
				logger.For(ctx).Warnf("notification create channel not open for user: %s", notif.OwnerID)
				n.UserUpdatedNotifications[notif.OwnerID] = nil
			}
		}
	})
	if err != nil {
		logger.For(nil).Errorf("error receiving new notifications from pubsub: %s", err)
		panic(err)
	}
}

func createNewData(oldData persist.NotificationData, newData persist.NotificationData) persist.NotificationData {
	// concat every array in newData to the corresponding array in oldData
	result := persist.NotificationData{}
	result.AdmirerIDs = append(oldData.AdmirerIDs, newData.AdmirerIDs...)
	result.FollowerIDs = append(oldData.FollowerIDs, newData.FollowerIDs...)
	result.ViewerIDs = append(oldData.ViewerIDs, newData.ViewerIDs...)

	return result
}
