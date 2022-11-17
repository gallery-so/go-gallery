package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/bsm/redislock"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

type lockKey struct {
	ownerID persist.DBID
	action  persist.Action
}

const viewWindow = 24 * time.Hour
const groupedWindow = 10 * time.Minute
const notificationTimeout = 10 * time.Second
const maxLockTimeout = 2 * time.Minute
const NotificationHandlerContextKey = "notification.notificationHandlers"

type NotificationHandlers struct {
	Notifications            *notificationDispatcher
	UserNewNotifications     map[persist.DBID]chan db.Notification
	UserUpdatedNotifications map[persist.DBID]chan db.Notification
	pubSub                   *pubsub.Client
}

// New registers specific notification handlers
func New(queries *db.Queries, pub *pubsub.Client, lock *redislock.Client) *NotificationHandlers {
	notifDispatcher := notificationDispatcher{handlers: map[persist.Action]notificationHandler{}, lock: lock}

	def := defaultNotificationHandler{queries: queries, pubSub: pub}
	group := groupedNotificationHandler{queries: queries, pubSub: pub}
	view := viewedNotificationHandler{queries: queries, pubSub: pub}

	// grouped notification actions
	notifDispatcher.AddHandler(persist.ActionUserFollowedUsers, group)
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
	lock     *redislock.Client
}

func (d *notificationDispatcher) AddHandler(action persist.Action, handler notificationHandler) {
	d.handlers[action] = handler
}

func (d *notificationDispatcher) Dispatch(ctx context.Context, notif db.Notification) error {
	if handler, ok := d.handlers[notif.Action]; ok {
		l, _ := d.lock.Obtain(ctx, lockKey{ownerID: notif.OwnerID, action: notif.Action}.String(), maxLockTimeout, &redislock.Options{RetryStrategy: redislock.LinearBackoff(5 * time.Second)})
		if l != nil {
			defer l.Release(ctx)
		}
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
	if time.Since(curNotif.CreatedAt) < groupedWindow {
		logger.For(ctx).Infof("grouping notification %s: %s-%s", curNotif.ID, notif.Action, notif.OwnerID)
		return updateAndPublishNotif(ctx, notif, curNotif, h.queries, h.pubSub)
	}
	logger.For(ctx).Infof("not grouping notification: %s-%s", notif.Action, notif.OwnerID)
	return insertAndPublishNotif(ctx, notif, h.queries, h.pubSub)

}

type viewedNotificationHandler struct {
	queries *coredb.Queries
	pubSub  *pubsub.Client
}

func beginningOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	pst, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}
	return time.Date(year, month, day, 0, 0, 0, 0, pst)
}

// this handler will still group notifications in the usual window, but it will also ensure that each viewer does
// does not show up mutliple times in a week
func (h viewedNotificationHandler) Handle(ctx context.Context, notif db.Notification) error {
	notifs, _ := h.queries.GetNotificationsByOwnerIDForActionAfter(ctx, db.GetNotificationsByOwnerIDForActionAfterParams{
		OwnerID:      notif.OwnerID,
		Action:       notif.Action,
		CreatedAfter: beginningOfDay(time.Now()),
	})
	if notifs == nil || len(notifs) == 0 {
		return insertAndPublishNotif(ctx, notif, h.queries, h.pubSub)
	}

	mostRecentNotif := notifs[0]

	if notif.Data.UnauthedViewerIDs != nil && len(notif.Data.UnauthedViewerIDs) > 0 {
		externalsToAdd := map[string]bool{}
		for _, id := range notif.Data.UnauthedViewerIDs {
			externalsToAdd[id] = true
		}
		for _, id := range notif.Data.UnauthedViewerIDs {
		firstInner:
			for _, n := range notifs {
				if util.ContainsString(n.Data.UnauthedViewerIDs, id) {
					externalsToAdd[id] = false
					break firstInner
				}
			}
		}
		resultIDs := []string{}
		for id, add := range externalsToAdd {
			if add {
				resultIDs = append(resultIDs, id)
			}
		}
		notif.Data.UnauthedViewerIDs = resultIDs
	}

	if notif.Data.AuthedViewerIDs != nil && len(notif.Data.AuthedViewerIDs) > 0 {
		idsToAdd := map[persist.DBID]bool{}
		for _, id := range notif.Data.AuthedViewerIDs {
			idsToAdd[id] = true
		}
		for _, id := range notif.Data.AuthedViewerIDs {
		secondInner:
			for _, n := range notifs {
				if persist.ContainsDBID(n.Data.AuthedViewerIDs, id) {
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
		notif.Data.AuthedViewerIDs = resultIDs
	}

	if time.Since(mostRecentNotif.CreatedAt) < viewWindow {
		return updateAndPublishNotif(ctx, notif, mostRecentNotif, h.queries, h.pubSub)
	}
	return insertAndPublishNotif(ctx, notif, h.queries, h.pubSub)
}

func (n *NotificationHandlers) receiveNewNotificationsFromPubSub() {
	sub, err := n.pubSub.CreateSubscription(context.Background(), fmt.Sprintf("new-notifications-%s", persist.GenerateID()), pubsub.SubscriptionConfig{
		Topic: n.pubSub.Topic(viper.GetString("PUBSUB_TOPIC_NEW_NOTIFICATIONS")),
	})
	if err != nil {
		logger.For(nil).Errorf("error creating updated notifications subscription: %s", err)
		panic(err)
	}

	logger.For(nil).Info("subscribing to new notifications pubsub topic")

	err = sub.Receive(context.Background(), func(ctx context.Context, msg *pubsub.Message) {

		logger.For(ctx).Debugf("received new notification from pubsub: %s", string(msg.Data))

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
	sub, err := n.pubSub.CreateSubscription(context.Background(), fmt.Sprintf("updated-notifications-%s", persist.GenerateID()), pubsub.SubscriptionConfig{
		Topic: n.pubSub.Topic(viper.GetString("PUBSUB_TOPIC_UPDATED_NOTIFICATIONS")),
	})
	if err != nil {
		logger.For(nil).Errorf("error creating updated notifications subscription: %s", err)
		panic(err)
	}

	logger.For(nil).Infof("subscribed to updated notifications pubsub")

	err = sub.Receive(context.Background(), func(ctx context.Context, msg *pubsub.Message) {

		logger.For(ctx).Debugf("received updated notification from pubsub: %s", string(msg.Data))

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
	newNotif, err := addNotification(ctx, notif, queries)
	if err != nil {
		return fmt.Errorf("failed to create notification: %w", err)
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
		return fmt.Errorf("failed to publish new notification: %w", err)
	}

	logger.For(ctx).Infof("pushed new notification to pubsub: %s", notif.OwnerID)
	return nil
}

func updateAndPublishNotif(ctx context.Context, notif db.Notification, mostRecentNotif db.Notification, queries *db.Queries, ps *pubsub.Client) error {
	amount := notif.Amount
	resultData := mostRecentNotif.Data.Concat(notif.Data)
	switch notif.Action {
	case persist.ActionAdmiredFeedEvent:
		amount = int32(len(resultData.AdmirerIDs))
	case persist.ActionViewedGallery:
		amount = int32(len(resultData.AuthedViewerIDs) + len(resultData.UnauthedViewerIDs))
	case persist.ActionUserFollowedUsers:
		amount = int32(len(resultData.FollowerIDs))
	default:
		amount = mostRecentNotif.Amount + notif.Amount
	}
	err := queries.UpdateNotification(ctx, db.UpdateNotificationParams{
		ID: mostRecentNotif.ID,
		// this concat will put the notif.Data values at the beginning of the array, sorted from most recently added to oldest added
		Data:   resultData,
		Amount: amount,
		// this will also get concatenated at the DB level
		EventIds: notif.EventIds,
	})
	if err != nil {
		return fmt.Errorf("error updating notification: %w", err)
	}
	updatedNotif, err := queries.GetNotificationByID(ctx, mostRecentNotif.ID)
	if err != nil {
		return fmt.Errorf("error getting updated notification by %s: %w", mostRecentNotif.ID, err)
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
		return fmt.Errorf("error publishing updated notification: %w", err)
	}

	logger.For(ctx).Infof("pushed updated notification to pubsub: %s", updatedNotif.OwnerID)
	return nil
}

func addNotification(ctx context.Context, notif db.Notification, queries *db.Queries) (db.Notification, error) {
	id := persist.GenerateID()
	switch notif.Action {
	case persist.ActionAdmiredFeedEvent:
		return queries.CreateAdmireNotification(ctx, db.CreateAdmireNotificationParams{
			ID:          id,
			OwnerID:     notif.OwnerID,
			Action:      notif.Action,
			Data:        notif.Data,
			EventIds:    notif.EventIds,
			FeedEventID: notif.FeedEventID,
		})
	case persist.ActionCommentedOnFeedEvent:
		return queries.CreateCommentNotification(ctx, db.CreateCommentNotificationParams{
			ID:          id,
			OwnerID:     notif.OwnerID,
			Action:      notif.Action,
			Data:        notif.Data,
			EventIds:    notif.EventIds,
			FeedEventID: notif.FeedEventID,
			CommentID:   notif.CommentID,
		})

	case persist.ActionUserFollowedUsers:
		return queries.CreateFollowNotification(ctx, db.CreateFollowNotificationParams{
			ID:       id,
			OwnerID:  notif.OwnerID,
			Action:   notif.Action,
			Data:     notif.Data,
			EventIds: notif.EventIds,
		})
	case persist.ActionViewedGallery:
		return queries.CreateViewGalleryNotification(ctx, db.CreateViewGalleryNotificationParams{
			ID:        id,
			OwnerID:   notif.OwnerID,
			Action:    notif.Action,
			Data:      notif.Data,
			EventIds:  notif.EventIds,
			GalleryID: notif.GalleryID,
		})

	default:
		return db.Notification{}, fmt.Errorf("unknown notification action: %s", notif.Action)
	}
}

func (l lockKey) String() string {
	return fmt.Sprintf("%s:%s", l.ownerID, l.action)
}
