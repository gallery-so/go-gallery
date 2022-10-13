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
	Notifications            *notificationDispatcher
	UserNewNotifications     map[persist.DBID]chan db.Notification
	UserUpdatedNotifications map[persist.DBID]chan db.Notification
	pubSub                   *pubsub.Client
}

// New registers specific notification handlers
func New(queries *db.Queries, pub *pubsub.Client) *NotificationHandlers {
	notifDispatcher := notificationDispatcher{handlers: map[persist.Action]notificationHandler{}}

	def := defaultNotificationHandler{queries: queries, pubSub: pub}
	group := groupedNotificationHandler{queries: queries, pubSub: pub}
	view := viewedNotificationHandler{queries: queries, pubSub: pub}

	// grouped notification actions
	notifDispatcher.AddHandler(persist.ActionUserFollowedUsers, group)
	notifDispatcher.AddHandler(persist.ActionUserFollowedUserBack, group)
	notifDispatcher.AddHandler(persist.ActionAdmiredFeedEvent, group)

	// single notification actions (default)
	notifDispatcher.AddHandler(persist.ActionCommentedOnFeedEvent, def)

	// viewed notifications are handled separately
	notifDispatcher.AddHandler(persist.ActionViewedGallery, view)

	new := map[persist.DBID]chan db.Notification{}
	updated := map[persist.DBID]chan db.Notification{}

	notificationHandlers := &NotificationHandlers{Notifications: &notifDispatcher, UserNewNotifications: new, UserUpdatedNotifications: updated, pubSub: pub}
	go notificationHandlers.receiveNewNotificationsFromPubSub()
	go notificationHandlers.receiveUpdatedNotificationsFromPubSub()
	return notificationHandlers
}

// Register specific notification handlers
func AddTo(ctx *gin.Context, notificationHandlers *NotificationHandlers) {
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
	if sub, ok := n.UserNewNotifications[userID]; ok && sub != nil {
		logger.For(context.Background()).Infof("returning existing new notification channel for user: %s", userID)
		return sub
	}
	sub := make(chan db.Notification)
	n.UserNewNotifications[userID] = sub
	logger.For(context.Background()).Infof("created new new notification channel for user: %s", userID)
	return sub
}

func (n *NotificationHandlers) GetUpdatedNotificationsForUser(userID persist.DBID) chan db.Notification {
	if sub, ok := n.UserUpdatedNotifications[userID]; ok && sub != nil {
		logger.For(context.Background()).Infof("returning existing updated notification channel for user: %s", userID)
		return sub
	}
	sub := make(chan db.Notification)
	n.UserUpdatedNotifications[userID] = sub
	logger.For(context.Background()).Infof("created new updated notification channel for user: %s", userID)
	return sub
}

func (n *NotificationHandlers) UnsubscribeNewNotificationsForUser(userID persist.DBID) {
	logger.For(context.Background()).Infof("unsubscribing new notifications for user: %s", userID)
	delete(n.UserNewNotifications, userID)
}

func (n *NotificationHandlers) UnsubscribeUpdatedNotificationsForUser(userID persist.DBID) {
	logger.For(context.Background()).Infof("unsubscribing updated notifications for user: %s", userID)
	delete(n.UserUpdatedNotifications, userID)
}

type notificationHandler interface {
	Handle(context.Context, db.Notification) error
}

type notificationDispatcher struct {
	handlers map[persist.Action]notificationHandler
}

func (d *notificationDispatcher) AddHandler(action persist.Action, handler notificationHandler) {
	d.handlers[action] = handler
}

func (d *notificationDispatcher) Dispatch(ctx context.Context, notif db.Notification) error {
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
	return insertAndPublishNotif(ctx, notif, h.queries, h.pubSub)
}

type groupedNotificationHandler struct {
	queries *coredb.Queries
	pubSub  *pubsub.Client
}

func (h groupedNotificationHandler) Handle(ctx context.Context, notif db.Notification) error {

	curNotif, _ := h.queries.GetMostRecentNotificationByOwnerIDForAction(ctx, db.GetMostRecentNotificationByOwnerIDForActionParams{
		OwnerID: notif.OwnerID,
		Action:  notif.Action,
	})
	if time.Since(curNotif.CreatedAt) < window {
		return updateAndPublishNotif(ctx, curNotif, notif, h.queries, h.pubSub)
	}
	return insertAndPublishNotif(ctx, notif, h.queries, h.pubSub)

}

type viewedNotificationHandler struct {
	queries *coredb.Queries
	pubSub  *pubsub.Client
}

// one week
const viewedNotificationUniqueViewerWindow = time.Hour * 24 * 7

// this handler will still group notifications in the usual window, but it will also ensure that each viewer does
// does not show up mutliple times in a week
func (h viewedNotificationHandler) Handle(ctx context.Context, notif db.Notification) error {

	notifs, _ := h.queries.GetNotificationsByOwnerIDForActionAfter(ctx, db.GetNotificationsByOwnerIDForActionAfterParams{
		OwnerID:      notif.OwnerID,
		Action:       notif.Action,
		CreatedAfter: time.Now().Add(-viewedNotificationUniqueViewerWindow),
	})
	if notifs == nil || len(notifs) == 0 {
		return insertAndPublishNotif(ctx, notif, h.queries, h.pubSub)
	}

	mostRecentNotif := notifs[0]

	if notif.Data.ViewerIPs != nil && len(notif.Data.ViewerIPs) > 0 {
		ipsToAdd := map[string]bool{}
		for _, ip := range notif.Data.ViewerIPs {
			ipsToAdd[ip] = true
		}
		for _, ip := range notif.Data.ViewerIPs {
		firstInner:
			for _, n := range notifs {
				if util.ContainsString(n.Data.ViewerIPs, ip) {
					ipsToAdd[ip] = false
					break firstInner
				}
			}
		}
		resultIPs := []string{}
		for ip, add := range ipsToAdd {
			if add {
				resultIPs = append(resultIPs, ip)
			}
		}
		notif.Data.ViewerIPs = resultIPs
	}

	if notif.Data.ViewerIDs != nil && len(notif.Data.ViewerIDs) > 0 {
		idsToAdd := map[persist.DBID]bool{}
		for _, id := range notif.Data.ViewerIDs {
			idsToAdd[id] = true
		}
		for _, id := range notif.Data.ViewerIDs {
		secondInner:
			for _, n := range notifs {
				if persist.ContainsDBID(n.Data.ViewerIDs, id) {
					idsToAdd[id] = false
					break secondInner
				}
			}
		}
		resultIDs := []persist.DBID{}
		for id, add := range idsToAdd {
			if add {
				resultIDs = append(resultIDs, id)
			}
		}
		notif.Data.ViewerIDs = resultIDs
	}

	if time.Since(mostRecentNotif.CreatedAt) < window {
		return updateAndPublishNotif(ctx, notif, mostRecentNotif, h.queries, h.pubSub)
	}
	return insertAndPublishNotif(ctx, notif, h.queries, h.pubSub)
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

		logger.For(ctx).Infof("received new notification from pubsub: %s", notif.OwnerID)

		if sub, ok := n.UserNewNotifications[notif.OwnerID]; ok && sub != nil {
			select {
			case sub <- notif:
				logger.For(ctx).Debugf("sent new notification to user: %s", notif.OwnerID)
			case <-time.After(notificationTimeout):
				logger.For(ctx).Debugf("notification create channel not open for user: %s", notif.OwnerID)
				n.UnsubscribeNewNotificationsForUser(notif.OwnerID)
			}
		} else {
			logger.For(ctx).Debugf("no notification create channel open for user: %s", notif.OwnerID)
		}
	})
	if err != nil {
		logger.For(nil).Errorf("error receiving new notifications from pubsub: %s", err)
		panic(err)
	}
}

func (n *NotificationHandlers) receiveUpdatedNotificationsFromPubSub() {
	sub := n.pubSub.Subscription(viper.GetString("PUBSUB_SUB_UPDATED_NOTIFICATIONS"))

	err := sub.Receive(context.Background(), func(ctx context.Context, msg *pubsub.Message) {

		defer msg.Ack()
		notif := db.Notification{}
		err := json.Unmarshal(msg.Data, &notif)
		if err != nil {
			logger.For(ctx).Warnf("failed to unmarshal pubsub message: %s", err)
			return
		}

		logger.For(ctx).Infof("received updated notification from pubsub: %s", notif.OwnerID)

		if sub, ok := n.UserUpdatedNotifications[notif.OwnerID]; ok && sub != nil {
			select {
			case sub <- notif:
				logger.For(ctx).Debugf("sent updated notification to user: %s", notif.OwnerID)
			case <-time.After(notificationTimeout):
				logger.For(ctx).Debugf("notification update channel not open for user: %s", notif.OwnerID)
				n.UnsubscribeUpdatedNotificationsForUser(notif.OwnerID)
			}
		} else {
			logger.For(ctx).Debugf("no notification update channel open for user: %s", notif.OwnerID)
		}
	})
	if err != nil {
		logger.For(nil).Errorf("error receiving new notifications from pubsub: %s", err)
		panic(err)
	}
}

func insertAndPublishNotif(ctx context.Context, notif db.Notification, queries *db.Queries, ps *pubsub.Client) error {
	newNotif, err := queries.CreateNotification(ctx, db.CreateNotificationParams{
		ID:      persist.GenerateID(),
		OwnerID: notif.OwnerID,
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
	t := ps.Topic(viper.GetString("PUBSUB_TOPIC_NEW_NOTIFICATIONS"))
	result := t.Publish(ctx, &pubsub.Message{
		Data: marshalled,
	})

	_, err = result.Get(ctx)
	if err != nil {
		return err
	}

	logger.For(ctx).Infof("pushed new notification to pubsub: %s", notif.OwnerID)
	return nil
}

func updateAndPublishNotif(ctx context.Context, notif db.Notification, mostRecentNotif db.Notification, queries *db.Queries, ps *pubsub.Client) error {
	amount := notif.Amount
	if amount < 1 {
		amount = 1
	}
	err := queries.UpdateNotification(ctx, db.UpdateNotificationParams{
		ID: mostRecentNotif.ID,
		// this concat will put the notif.Data values at the beginning of the array, sorted from most recently added to oldest added
		Data:   mostRecentNotif.Data.Concat(notif.Data),
		Amount: amount,
	})
	if err != nil {
		return err
	}
	updatedNotif, err := queries.GetNotificationByID(ctx, mostRecentNotif.ID)
	if err != nil {
		return err
	}
	marshalled, err := json.Marshal(updatedNotif)
	if err != nil {
		return err
	}
	t := ps.Topic(viper.GetString("PUBSUB_TOPIC_UPDATED_NOTIFICATIONS"))
	result := t.Publish(ctx, &pubsub.Message{
		Data: marshalled,
	})
	_, err = result.Get(ctx)
	if err != nil {
		return err
	}

	logger.For(ctx).Infof("pushed updated notification to pubsub: %s", updatedNotif.OwnerID)
	return nil
}
