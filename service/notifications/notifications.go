package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/bsm/redislock"
	"github.com/gin-gonic/gin"
	"github.com/googleapis/gax-go/v2/apierror"
	"github.com/sourcegraph/conc/pool"
	"golang.org/x/net/html"
	"google.golang.org/grpc/codes"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/limiters"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/redis"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
)

type lockKey struct {
	ownerID persist.DBID
	action  persist.Action
}

const viewWindow = 24 * time.Hour
const defaultGroupedWindow = 10 * time.Minute
const notificationTimeout = 10 * time.Second
const maxLockTimeout = 2 * time.Minute
const NotificationHandlerContextKey = "notification.notificationHandlers"

var notificationTypeToGroupWindow = map[persist.Action]time.Duration{
	persist.ActionAdmiredComment: 24 * time.Hour,
	persist.ActionAdmiredPost:    24 * time.Hour,
	persist.ActionAdmiredToken:   24 * time.Hour,
}

type NotificationHandlers struct {
	Notifications            *notificationDispatcher
	UserNewNotifications     map[persist.DBID]chan db.Notification
	UserUpdatedNotifications map[persist.DBID]chan db.Notification
	pubSub                   *pubsub.Client
}

type pushLimiter struct {
	comments     *limiters.KeyRateLimiter
	admires      *limiters.KeyRateLimiter
	follows      *limiters.KeyRateLimiter
	tokens       *limiters.KeyRateLimiter
	feedEntities *limiters.KeyRateLimiter
	users        *limiters.KeyRateLimiter
}

func newPushLimiter() *pushLimiter {
	cache := redis.NewCache(redis.PushNotificationRateLimitersCache)
	ctx := context.Background()
	return &pushLimiter{
		comments:     limiters.NewKeyRateLimiter(ctx, cache, "comments", 5, time.Minute),
		admires:      limiters.NewKeyRateLimiter(ctx, cache, "admires", 1, time.Minute*10),
		follows:      limiters.NewKeyRateLimiter(ctx, cache, "follows", 1, time.Minute*10),
		tokens:       limiters.NewKeyRateLimiter(ctx, cache, "tokens", 1, time.Minute*10),
		feedEntities: limiters.NewKeyRateLimiter(ctx, cache, "feedEntities", 5, time.Minute),
		users:        limiters.NewKeyRateLimiter(ctx, cache, "users", 5, time.Minute),
	}
}

func (p *pushLimiter) tryComment(ctx context.Context, sendingUserID persist.DBID, receivingUserID persist.DBID, feedEventID persist.DBID) error {
	key := fmt.Sprintf("%s:%s:%s", sendingUserID.String(), receivingUserID.String(), feedEventID.String())
	if p.isActionAllowed(ctx, p.comments, key) {
		return nil
	}

	return errRateLimited{
		limiterName: p.comments.Name(),
		senderID:    sendingUserID,
		receiverID:  receivingUserID,
		feedEventID: feedEventID,
	}
}

func (p *pushLimiter) tryFeedEntity(ctx context.Context, sendingUserID persist.DBID, receivingUserID persist.DBID, postID persist.DBID) error {
	key := fmt.Sprintf("%s:%s:%s", sendingUserID.String(), receivingUserID.String(), postID.String())
	if p.isActionAllowed(ctx, p.feedEntities, key) {
		return nil
	}

	return errRateLimited{
		limiterName: p.feedEntities.Name(),
		senderID:    sendingUserID,
		receiverID:  receivingUserID,
		feedEventID: postID,
	}
}

func (p *pushLimiter) tryAdmire(ctx context.Context, sendingUserID persist.DBID, receivingUserID persist.DBID, feedEventID persist.DBID) error {
	key := fmt.Sprintf("%s:%s:%s", sendingUserID.String(), receivingUserID.String(), feedEventID.String())
	if p.isActionAllowed(ctx, p.admires, key) {
		return nil
	}

	return errRateLimited{
		limiterName: p.admires.Name(),
		senderID:    sendingUserID,
		receiverID:  receivingUserID,
		feedEventID: feedEventID,
	}
}

func (p *pushLimiter) tryAdmireComment(ctx context.Context, sendingUserID persist.DBID, receivingUserID persist.DBID, commentID persist.DBID) error {
	key := fmt.Sprintf("%s:%s:%s", sendingUserID.String(), receivingUserID.String(), commentID.String())
	if p.isActionAllowed(ctx, p.admires, key) {
		return nil
	}

	return errRateLimited{
		limiterName: p.admires.Name(),
		senderID:    sendingUserID,
		receiverID:  receivingUserID,
		commentID:   commentID,
	}
}

func (p *pushLimiter) tryAdmireToken(ctx context.Context, sendingUserID persist.DBID, receivingUserID persist.DBID, tokenID persist.DBID) error {
	key := fmt.Sprintf("%s:%s:%s", sendingUserID.String(), receivingUserID.String(), tokenID.String())
	if p.isActionAllowed(ctx, p.admires, key) {
		return nil
	}

	return errRateLimited{
		limiterName: p.admires.Name(),
		senderID:    sendingUserID,
		receiverID:  receivingUserID,
		tokenID:     tokenID,
	}
}

func (p *pushLimiter) tryFollow(ctx context.Context, sendingUserID persist.DBID, receivingUserID persist.DBID) error {
	key := fmt.Sprintf("%s:%s", sendingUserID, receivingUserID)
	if p.isActionAllowed(ctx, p.follows, key) {
		return nil
	}

	return errRateLimited{
		limiterName: p.follows.Name(),
		senderID:    sendingUserID,
		receiverID:  receivingUserID,
	}
}

func (p *pushLimiter) tryTokens(ctx context.Context, sendingUserID persist.DBID, tokenID persist.DBID) error {
	key := fmt.Sprintf("%s:%s", sendingUserID, tokenID)
	if p.isActionAllowed(ctx, p.tokens, key) {
		return nil
	}

	return errRateLimited{
		limiterName: p.follows.Name(),
		senderID:    sendingUserID,
	}
}

func (p *pushLimiter) tryUsers(ctx context.Context, sendingUserID persist.DBID) error {
	key := fmt.Sprintf("%s", sendingUserID)
	if p.isActionAllowed(ctx, p.users, key) {
		return nil
	}

	return errRateLimited{
		limiterName: p.users.Name(),
		senderID:    sendingUserID,
	}
}

func (p *pushLimiter) isActionAllowed(ctx context.Context, limiter *limiters.KeyRateLimiter, key string) bool {
	canContinue, _, err := limiter.ForKey(ctx, key)
	if err != nil {
		logger.For(ctx).Warnf("error getting rate limit for key %s: %s", key, err.Error())
		return false
	}

	return canContinue
}

// New registers specific notification handlers
func New(queries *db.Queries, pub *pubsub.Client, taskClient *task.Client, lock *redislock.Client, listen bool) *NotificationHandlers {
	notifDispatcher := notificationDispatcher{handlers: map[persist.Action]notificationHandler{}, lock: lock}
	limiter := newPushLimiter()

	singleHandler := singleNotificationHandler{queries: queries, pubSub: pub, taskClient: taskClient, limiter: limiter}
	ownerGroupedHandler := ownerGroupedNotificationHandler{queries: queries, pubSub: pub, taskClient: taskClient, limiter: limiter}
	tokenGroupedHandler := tokenIDGroupedNotificationHandler{queries: queries, pubSub: pub, taskClient: taskClient, limiter: limiter}
	viewHandler := viewedNotificationHandler{queries: queries, pubSub: pub, taskClient: taskClient, limiter: limiter}
	topActivityHandler := topActivityHandler{queries: queries, pubSub: pub, taskClient: taskClient, limiter: limiter}
	announcementHandler := announcementNotificationHandler{queries: queries, pubSub: pub, taskClient: taskClient, limiter: limiter}

	// notification actions that are grouped by owner
	notifDispatcher.AddHandler(persist.ActionUserFollowedUsers, ownerGroupedHandler)
	notifDispatcher.AddHandler(persist.ActionAdmiredFeedEvent, ownerGroupedHandler)
	notifDispatcher.AddHandler(persist.ActionAdmiredPost, ownerGroupedHandler)
	notifDispatcher.AddHandler(persist.ActionAdmiredComment, ownerGroupedHandler)

	// single notification actions
	notifDispatcher.AddHandler(persist.ActionCommentedOnFeedEvent, singleHandler)
	notifDispatcher.AddHandler(persist.ActionCommentedOnPost, singleHandler)
	notifDispatcher.AddHandler(persist.ActionReplyToComment, singleHandler)
	notifDispatcher.AddHandler(persist.ActionMentionUser, singleHandler)
	notifDispatcher.AddHandler(persist.ActionMentionCommunity, singleHandler)
	notifDispatcher.AddHandler(persist.ActionUserPostedYourWork, singleHandler)
	notifDispatcher.AddHandler(persist.ActionUserFromFarcasterJoined, singleHandler)
	notifDispatcher.AddHandler(persist.ActionUserPostedFirstPost, singleHandler)

	// notification specifically for ensuring that top users don't get re-notified and users recently notified arent notified again
	notifDispatcher.AddHandler(persist.ActionTopActivityBadgeReceived, topActivityHandler)

	// notification actions that are grouped and inserted by follower

	// notification actions that are grouped by token id
	notifDispatcher.AddHandler(persist.ActionNewTokensReceived, tokenGroupedHandler)
	notifDispatcher.AddHandler(persist.ActionAdmiredToken, tokenGroupedHandler)

	// viewed notifications are handled separately
	notifDispatcher.AddHandler(persist.ActionViewedGallery, viewHandler)

	// announcements are handled separately
	notifDispatcher.AddHandler(persist.ActionAnnouncement, announcementHandler)

	new := map[persist.DBID]chan db.Notification{}
	updated := map[persist.DBID]chan db.Notification{}

	notificationHandlers := &NotificationHandlers{Notifications: &notifDispatcher, UserNewNotifications: new, UserUpdatedNotifications: updated, pubSub: pub}
	if pub != nil && listen {
		go notificationHandlers.receiveNewNotificationsFromPubSub()
		go notificationHandlers.receiveUpdatedNotificationsFromPubSub()
	} else {
		logger.For(nil).Warn("pubsub not configured, notifications will not be received")
	}
	return notificationHandlers
}

// Register specific notification handlers
func AddTo(ctx *gin.Context, notificationHandlers *NotificationHandlers) {
	ctx.Set(NotificationHandlerContextKey, notificationHandlers)
}

func For(ctx context.Context) *NotificationHandlers {
	gc := util.MustGetGinContext(ctx)
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

type singleNotificationHandler struct {
	queries    *db.Queries
	pubSub     *pubsub.Client
	taskClient *task.Client
	limiter    *pushLimiter
}

func (h singleNotificationHandler) Handle(ctx context.Context, notif db.Notification) error {
	return insertAndPublishNotif(ctx, notif, h.queries, h.pubSub, h.taskClient, h.limiter)
}

type topActivityHandler struct {
	queries    *db.Queries
	pubSub     *pubsub.Client
	taskClient *task.Client
	limiter    *pushLimiter
}

func (h topActivityHandler) Handle(ctx context.Context, notif db.Notification) error {
	if !notif.Data.NewTopActiveUser {
		return nil
	}

	return insertAndPublishNotif(ctx, notif, h.queries, h.pubSub, h.taskClient, h.limiter)
}

type announcementNotificationHandler struct {
	queries    *db.Queries
	pubSub     *pubsub.Client
	taskClient *task.Client
	limiter    *pushLimiter
}

func (h announcementNotificationHandler) Handle(ctx context.Context, notif db.Notification) error {
	return insertAndPublishAnnouncementNotifs(ctx, notif, h.queries, h.pubSub, h.taskClient, h.limiter)
}

type ownerGroupedNotificationHandler struct {
	queries    *db.Queries
	pubSub     *pubsub.Client
	taskClient *task.Client
	limiter    *pushLimiter
}

func (h ownerGroupedNotificationHandler) Handle(ctx context.Context, notif db.Notification) error {
	var curNotif db.Notification
	// Bucket notifications on a specific resource if it has one
	onlyForFeed := notif.FeedEventID != ""
	onlyForPost := notif.PostID != ""
	onlyForComment := notif.CommentID != ""
	curNotif, _ = h.queries.GetMostRecentNotificationByOwnerIDForAction(ctx, db.GetMostRecentNotificationByOwnerIDForActionParams{
		OwnerID:          notif.OwnerID,
		Action:           notif.Action,
		OnlyForFeedEvent: onlyForFeed,
		FeedEventID:      notif.FeedEventID,
		PostID:           notif.PostID,
		OnlyForPost:      onlyForPost,
		CommentID:        notif.CommentID,
		OnlyForComment:   onlyForComment,
	})

	window, ok := notificationTypeToGroupWindow[notif.Action]
	if !ok {
		window = defaultGroupedWindow
	}

	if time.Since(curNotif.CreatedAt) < window {
		logger.For(ctx).Infof("grouping notification %s: %s-%s", curNotif.ID, notif.Action, notif.OwnerID)
		return updateAndPublishNotif(ctx, notif, curNotif, h.queries, h.pubSub, h.taskClient, h.limiter)
	}
	logger.For(ctx).Infof("not grouping notification: %s-%s", notif.Action, notif.OwnerID)
	return insertAndPublishNotif(ctx, notif, h.queries, h.pubSub, h.taskClient, h.limiter)
}

type tokenIDGroupedNotificationHandler struct {
	queries    *db.Queries
	pubSub     *pubsub.Client
	taskClient *task.Client
	limiter    *pushLimiter
}

func (h tokenIDGroupedNotificationHandler) Handle(ctx context.Context, notif db.Notification) error {
	var curNotif db.Notification

	// Bucket notifications on the feed event if it has one
	onlyForFeed := notif.FeedEventID != ""
	onlyForPost := notif.PostID != ""
	curNotif, _ = h.queries.GetMostRecentNotificationByOwnerIDTokenIDForAction(ctx, db.GetMostRecentNotificationByOwnerIDTokenIDForActionParams{
		OwnerID:          notif.OwnerID,
		Action:           notif.Action,
		TokenID:          notif.TokenID,
		OnlyForFeedEvent: onlyForFeed,
		FeedEventID:      notif.FeedEventID,
		PostID:           notif.PostID,
		OnlyForPost:      onlyForPost,
	})

	window, ok := notificationTypeToGroupWindow[notif.Action]
	if !ok {
		window = defaultGroupedWindow
	}

	if time.Since(curNotif.CreatedAt) < window {
		logger.For(ctx).Infof("grouping notification %s: %s-%s", curNotif.ID, notif.Action, notif.OwnerID)
		return updateAndPublishNotif(ctx, notif, curNotif, h.queries, h.pubSub, h.taskClient, h.limiter)
	}
	logger.For(ctx).Infof("not grouping notification: %s-%s", notif.Action, notif.OwnerID)
	return insertAndPublishNotif(ctx, notif, h.queries, h.pubSub, h.taskClient, h.limiter)

}

type viewedNotificationHandler struct {
	queries    *db.Queries
	pubSub     *pubsub.Client
	taskClient *task.Client
	limiter    *pushLimiter
}

// will return the beginning of the week (sunday) in PST
func beginningOfWeek(t time.Time) time.Time {

	pst, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}

	y, m, d := t.In(pst).Date()

	newD := d - int(t.Weekday())

	return time.Date(y, m, newD, 0, 0, 0, 0, pst)
}

// this handler will still group notifications in the usual window, but it will also ensure that each viewer does
// does not show up mutliple times in a week
func (h viewedNotificationHandler) Handle(ctx context.Context, notif db.Notification) error {
	// all of this user's view notifications in the current week
	notifs, _ := h.queries.GetNotificationsByOwnerIDForActionAfter(ctx, db.GetNotificationsByOwnerIDForActionAfterParams{
		OwnerID:      notif.OwnerID,
		Action:       notif.Action,
		CreatedAfter: beginningOfWeek(time.Now()),
	})
	if len(notifs) == 0 {
		// if there are no notifications this week, then we definitely are going to insert this one
		logger.For(ctx).Debugf("no notifications this week, inserting: %s-%s", notif.Action, notif.OwnerID)
		return insertAndPublishNotif(ctx, notif, h.queries, h.pubSub, h.taskClient, h.limiter)
	}

	mostRecentNotif := notifs[0]

	if notif.Data.UnauthedViewerIDs != nil && len(notif.Data.UnauthedViewerIDs) > 0 {

		resultIDs := []string{}
		// add each of the unauthed viewer ids in the passed in notif to the map unless it is already in one of the notifications this week
		for _, id := range notif.Data.UnauthedViewerIDs {
			add := true
		firstInner:
			for _, n := range notifs {
				if util.ContainsString(n.Data.UnauthedViewerIDs, id) {
					add = false
					break firstInner
				}
			}
			if add {
				resultIDs = append(resultIDs, id)
			}
		}

		notif.Data.UnauthedViewerIDs = resultIDs
	}

	if notif.Data.AuthedViewerIDs != nil && len(notif.Data.AuthedViewerIDs) > 0 {
		// go through each of the authed viewer ids in the passed in notif and add them to the map unless they are already in one of the notifications this week
		resultIDs := []persist.DBID{}
		for _, id := range notif.Data.AuthedViewerIDs {
			add := true
		secondInner:
			for _, n := range notifs {
				if persist.ContainsDBID(n.Data.AuthedViewerIDs, id) {
					add = false
					break secondInner
				}
			}
			if add {
				resultIDs = append(resultIDs, id)
			}
		}

		notif.Data.AuthedViewerIDs = resultIDs
	}

	// if the most recent notification in the last week is within the grouping window then we will update it, if not, insert it
	if time.Since(mostRecentNotif.CreatedAt) < viewWindow {
		logger.For(ctx).Debugf("grouping notification %s: %s-%s", mostRecentNotif.ID, notif.Action, notif.OwnerID)
		return updateAndPublishNotif(ctx, notif, mostRecentNotif, h.queries, h.pubSub, h.taskClient, h.limiter)
	}
	logger.For(ctx).Debugf("not grouping notification: %s-%s", notif.Action, notif.OwnerID)
	return insertAndPublishNotif(ctx, notif, h.queries, h.pubSub, h.taskClient, h.limiter)
}

// subscribe returns a subscription to the given topic
func (n *NotificationHandlers) subscribe(ctx context.Context, topic, name string) (*pubsub.Subscription, error) {
	sub, err := createSubscription(ctx, n.pubSub, topic, name)
	if err == nil {
		return sub, nil
	}

	if errTopicMissing(err) {
		if _, err := n.pubSub.CreateTopic(ctx, topic); err != nil {
			return nil, err
		}
	}

	return createSubscription(ctx, n.pubSub, topic, name)
}

func (n *NotificationHandlers) receiveNewNotificationsFromPubSub() {
	sub, err := n.subscribe(context.Background(), env.GetString("PUBSUB_TOPIC_NEW_NOTIFICATIONS"), fmt.Sprintf("new-notifications-%s", persist.GenerateID()))
	if err != nil {
		logger.For(nil).Errorf("error creating new notifications subscription: %s", err)
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
	sub, err := n.subscribe(context.Background(), env.GetString("PUBSUB_TOPIC_UPDATED_NOTIFICATIONS"), fmt.Sprintf("updated-notifications-%s", persist.GenerateID()))
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

func createPushMessage(ctx context.Context, notif db.Notification, queries *db.Queries, limiter *pushLimiter) (task.PushNotificationMessage, error) {
	badgeCount, badgeErr := queries.CountUserUnseenNotifications(ctx, notif.OwnerID)
	if badgeErr != nil {
		return task.PushNotificationMessage{}, badgeErr
	}

	message := task.PushNotificationMessage{
		Title: "Gallery",
		Sound: true,
		Badge: int(badgeCount),
		Data: map[string]any{
			"action":          notif.Action,
			"notification_id": notif.ID,
		},
	}

	userFacing, err := NotificationToUserFacingData(ctx, queries, notif)
	if err != nil {
		return task.PushNotificationMessage{}, err
	}

	message.Body = userFacing.String()

	switch {
	case notif.Action == persist.ActionAdmiredFeedEvent || notif.Action == persist.ActionAdmiredPost:
		admirer, err := queries.GetUserById(ctx, notif.Data.AdmirerIDs[0])
		if err != nil {
			return task.PushNotificationMessage{}, err
		}
		if err = limiter.tryAdmire(ctx, admirer.ID, notif.OwnerID, notif.FeedEventID); err != nil {
			return task.PushNotificationMessage{}, err
		}
		return message, nil
	case notif.Action == persist.ActionAdmiredToken:
		admirer, err := queries.GetUserById(ctx, notif.Data.AdmirerIDs[0])
		if err != nil {
			return task.PushNotificationMessage{}, err
		}
		if err = limiter.tryAdmireToken(ctx, admirer.ID, notif.OwnerID, notif.TokenID); err != nil {
			return task.PushNotificationMessage{}, err
		}
		return message, nil
	case notif.Action == persist.ActionAdmiredComment:
		admirer, err := queries.GetUserById(ctx, notif.Data.AdmirerIDs[0])
		if err != nil {
			return task.PushNotificationMessage{}, err
		}
		if err = limiter.tryAdmireComment(ctx, admirer.ID, notif.OwnerID, notif.TokenID); err != nil {
			return task.PushNotificationMessage{}, err
		}
		return message, nil
	case notif.Action == persist.ActionCommentedOnFeedEvent || notif.Action == persist.ActionCommentedOnPost:
		comment, err := queries.GetCommentByCommentID(ctx, notif.CommentID)
		if err != nil {
			return task.PushNotificationMessage{}, err
		}
		commenter, err := queries.GetUserById(ctx, comment.ActorID)
		if err != nil {
			return task.PushNotificationMessage{}, err
		}
		if err = limiter.tryComment(ctx, commenter.ID, notif.OwnerID, notif.FeedEventID); err != nil {
			return task.PushNotificationMessage{}, err
		}
		return message, nil
	case notif.Action == persist.ActionUserFollowedUsers:
		follower, err := queries.GetUserById(ctx, notif.Data.FollowerIDs[0])
		if err != nil {
			return task.PushNotificationMessage{}, err
		}
		if time.Since(follower.CreatedAt) < 24*time.Hour {
			return task.PushNotificationMessage{}, errAccountTooNew
		}
		if err = limiter.tryFollow(ctx, follower.ID, notif.OwnerID); err != nil {
			return task.PushNotificationMessage{}, err
		}
		return message, nil
	case notif.Action == persist.ActionNewTokensReceived:
		if err := limiter.tryTokens(ctx, notif.OwnerID, notif.Data.NewTokenID); err != nil {
			return task.PushNotificationMessage{}, err
		}
		return message, nil
	case notif.Action == persist.ActionReplyToComment:
		comment, err := queries.GetCommentByCommentID(ctx, notif.CommentID)
		if err != nil {
			return task.PushNotificationMessage{}, err
		}
		commenter, err := queries.GetUserById(ctx, comment.ActorID)
		if err != nil {
			return task.PushNotificationMessage{}, err
		}
		if err = limiter.tryFeedEntity(ctx, commenter.ID, notif.OwnerID, notif.PostID); err != nil {
			return task.PushNotificationMessage{}, err
		}
		return message, nil
	case notif.Action == persist.ActionMentionUser || notif.Action == persist.ActionMentionCommunity:
		var actor db.User
		switch {
		case notif.CommentID != "":
			comment, err := queries.GetCommentByCommentID(ctx, notif.CommentID)
			if err != nil {
				return task.PushNotificationMessage{}, err
			}
			actor, err = queries.GetUserById(ctx, comment.ActorID)
			if err != nil {
				return task.PushNotificationMessage{}, err
			}
		case notif.PostID != "":
			post, err := queries.GetPostByID(ctx, notif.PostID)
			if err != nil {
				return task.PushNotificationMessage{}, err
			}
			actor, err = queries.GetUserById(ctx, post.ActorID)
			if err != nil {
				return task.PushNotificationMessage{}, err
			}
		default:
			return task.PushNotificationMessage{}, fmt.Errorf("no comment or post id provided for mention notification")
		}
		if err := limiter.tryFeedEntity(ctx, actor.ID, notif.OwnerID, notif.PostID); err != nil {
			return task.PushNotificationMessage{}, err
		}
		return message, nil
	case notif.Action == persist.ActionUserPostedYourWork:
		post, err := queries.GetPostByID(ctx, notif.PostID)
		if err != nil {
			return task.PushNotificationMessage{}, err
		}
		actor, err := queries.GetUserById(ctx, post.ActorID)
		if err != nil {
			return task.PushNotificationMessage{}, err
		}
		if err := limiter.tryFeedEntity(ctx, actor.ID, notif.OwnerID, notif.PostID); err != nil {
			return task.PushNotificationMessage{}, err
		}
		return message, nil
	case notif.Action == persist.ActionTopActivityBadgeReceived:
		if err := limiter.tryUsers(ctx, notif.OwnerID); err != nil {
			return task.PushNotificationMessage{}, err
		}
		return message, nil
	case notif.Action == persist.ActionAnnouncement:
		// No rate limiting for announcements
		return message, nil
	case notif.Action == persist.ActionUserFromFarcasterJoined:
		// No rate limiting for farcaster joined
		return message, nil
	case notif.Action == persist.ActionUserPostedFirstPost:
		// No rate limiting for first post
		return message, nil
	default:
		return task.PushNotificationMessage{}, fmt.Errorf("unsupported notification action: %s", notif.Action)
	}
}

type UserFacingNotificationData struct {
	Actor          string       `json:"actor"`
	Action         string       `json:"action"`
	CollectionName string       `json:"collectionName"`
	CollectionID   persist.DBID `json:"collectionId"`
	PreviewText    string       `json:"previewText"`
}

func (u UserFacingNotificationData) String() string {
	cur := fmt.Sprintf("%s %s", u.Actor, u.Action)
	if u.CollectionName != "" {
		cur = fmt.Sprintf("%s %s", cur, u.CollectionName)
	}
	if u.PreviewText != "" {
		cur = fmt.Sprintf("%s: %s", cur, u.PreviewText)
	}
	return html.UnescapeString(cur)
}

var errNameNotAvailable = errors.New("name not available")

func NotificationToUserFacingData(ctx context.Context, queries *coredb.Queries, n coredb.Notification) (UserFacingNotificationData, error) {

	switch n.Action {
	case persist.ActionAdmiredFeedEvent, persist.ActionAdmiredPost, persist.ActionAdmiredToken, persist.ActionAdmiredComment:
		data := UserFacingNotificationData{}
		if n.Action == persist.ActionAdmiredFeedEvent {
			feedEvent, err := queries.GetFeedEventByID(ctx, n.FeedEventID)
			if err != nil {
				return UserFacingNotificationData{}, fmt.Errorf("failed to get feed event for admire %s: %w", n.FeedEventID, err)
			}
			collection, _ := queries.GetCollectionById(ctx, feedEvent.Data.CollectionID)
			if collection.ID != "" && collection.Name.String != "" {
				data.CollectionID = collection.ID
				data.CollectionName = collection.Name.String
				data.Action = "admired your additions to"
			} else {
				data.Action = "admired your gallery update"
			}
		} else if n.Action == persist.ActionAdmiredToken {
			data.Action = "bookmarked your token"
		} else if n.Action == persist.ActionAdmiredComment {
			data.Action = "admired your comment"
		} else {
			data.Action = "admired your post"
		}
		if len(n.Data.AdmirerIDs) > 1 {
			data.Actor = fmt.Sprintf("%d collectors", len(n.Data.AdmirerIDs))
		} else {
			actorUser, err := queries.GetUserById(ctx, n.Data.AdmirerIDs[0])
			if err != nil {
				return UserFacingNotificationData{}, err
			}
			data.Actor = actorUser.Username.String
		}
		return data, nil
	case persist.ActionUserFollowedUsers:
		if len(n.Data.FollowerIDs) > 1 {
			return UserFacingNotificationData{
				Actor:  fmt.Sprintf("%d users", len(n.Data.FollowerIDs)),
				Action: "followed you",
			}, nil
		}
		if len(n.Data.FollowerIDs) == 1 {
			userActor, err := queries.GetUserById(ctx, n.Data.FollowerIDs[0])
			if err != nil {
				return UserFacingNotificationData{}, fmt.Errorf("failed to get user for follower %s: %w", n.Data.FollowerIDs[0], err)
			}
			action := "followed you"
			if n.Data.FollowedBack {
				action = "followed you back"
			}
			return UserFacingNotificationData{
				Actor:  userActor.Username.String,
				Action: action,
			}, nil
		}
		return UserFacingNotificationData{}, fmt.Errorf("no follower ids")
	case persist.ActionCommentedOnFeedEvent, persist.ActionCommentedOnPost:
		comment, err := queries.GetCommentByCommentID(ctx, n.CommentID)
		if err != nil {
			return UserFacingNotificationData{}, fmt.Errorf("failed to get comment for comment %s: %w", n.CommentID, err)
		}
		userActor, err := queries.GetUserById(ctx, comment.ActorID)
		if err != nil {
			return UserFacingNotificationData{}, fmt.Errorf("failed to get user for comment actor %s: %w", comment.ActorID, err)
		}
		feedEvent, _ := queries.GetFeedEventByID(ctx, n.FeedEventID)
		action := "commented on your post"
		if n.Action == persist.ActionCommentedOnFeedEvent && feedEvent.Data.CollectionID != "" {
			collection, err := queries.GetCollectionById(ctx, feedEvent.Data.CollectionID)
			if err != nil {
				return UserFacingNotificationData{}, fmt.Errorf("failed to get collection for comment %s: %w", feedEvent.Data.CollectionID, err)
			}

			return UserFacingNotificationData{
				Actor:          userActor.Username.String,
				Action:         "commented on your additions to",
				CollectionName: collection.Name.String,
				CollectionID:   collection.ID,
				PreviewText:    util.TruncateWithEllipsis(comment.Comment, 40),
			}, nil

		} else if n.Action == persist.ActionCommentedOnFeedEvent {
			action = "commented on your gallery update"
		}

		return UserFacingNotificationData{
			Actor:       userActor.Username.String,
			Action:      action,
			PreviewText: util.TruncateWithEllipsis(comment.Comment, 40),
		}, nil
	case persist.ActionViewedGallery:
		if len(n.Data.AuthedViewerIDs)+len(n.Data.UnauthedViewerIDs) > 1 {
			return UserFacingNotificationData{
				Actor:  fmt.Sprintf("%d collectors", len(n.Data.AuthedViewerIDs)+len(n.Data.UnauthedViewerIDs)),
				Action: "viewed your gallery",
			}, nil
		}
		if len(n.Data.AuthedViewerIDs) == 1 {
			userActor, err := queries.GetUserById(ctx, n.Data.AuthedViewerIDs[0])
			if err != nil {
				return UserFacingNotificationData{}, fmt.Errorf("failed to get user for viewer %s: %w", n.Data.AuthedViewerIDs[0], err)
			}
			return UserFacingNotificationData{
				Actor:  userActor.Username.String,
				Action: "viewed your gallery",
			}, nil
		}
		if len(n.Data.UnauthedViewerIDs) == 1 {
			return UserFacingNotificationData{
				Actor:  "Someone",
				Action: "viewed your gallery",
			}, nil
		}

		return UserFacingNotificationData{}, fmt.Errorf("no viewer ids")

	case persist.ActionMentionUser, persist.ActionMentionCommunity:

		data := UserFacingNotificationData{}
		var actor db.User
		var mentionedIn string
		var preview string
		switch {
		case n.CommentID != "":

			mentionedIn = "comment"

			comment, err := queries.GetCommentByCommentID(ctx, n.CommentID)
			if err != nil {
				return UserFacingNotificationData{}, err
			}

			preview = util.TruncateWithEllipsis(comment.Comment, 40)

			actor, err = queries.GetUserById(ctx, comment.ActorID)
			if err != nil {
				return UserFacingNotificationData{}, err
			}

		case n.PostID != "":
			mentionedIn = "post"

			post, err := queries.GetPostByID(ctx, n.PostID)
			if err != nil {
				return UserFacingNotificationData{}, err
			}

			preview = util.TruncateWithEllipsis(post.Caption.String, 40)

			actor, err = queries.GetUserById(ctx, post.ActorID)
			if err != nil {
				return UserFacingNotificationData{}, err
			}

		default:
			return UserFacingNotificationData{}, fmt.Errorf("no comment or post id provided for mention notification")
		}
		if mentionedIn == "" || preview == "" {
			return UserFacingNotificationData{}, fmt.Errorf("no push data found for mention notification")
		}

		if !actor.Username.Valid {
			return UserFacingNotificationData{}, fmt.Errorf("user with ID=%s has no username", actor.ID)
		}

		data.Actor = actor.Username.String
		data.PreviewText = preview

		if n.Action == persist.ActionMentionCommunity {
			community, err := queries.GetCommunityByID(ctx, n.CommunityID)
			if err != nil {
				return UserFacingNotificationData{}, err
			}

			data.Action = fmt.Sprintf("mentioned your community @%s in a comment", community.Name)
		} else {
			data.Action = "mentioned you in a comment"
		}
		return data, nil
	case persist.ActionReplyToComment:

		comment, err := queries.GetCommentByCommentID(ctx, n.CommentID)
		if err != nil {
			return UserFacingNotificationData{}, err
		}

		commenter, err := queries.GetUserById(ctx, comment.ActorID)
		if err != nil {
			return UserFacingNotificationData{}, err
		}

		if !commenter.Username.Valid {
			return UserFacingNotificationData{}, fmt.Errorf("user with ID=%s has no username", commenter.ID)
		}

		return UserFacingNotificationData{
			Actor:       commenter.Username.String,
			Action:      "replied to your comment",
			PreviewText: util.TruncateWithEllipsis(comment.Comment, 40),
		}, nil
	case persist.ActionNewTokensReceived:
		data := UserFacingNotificationData{}

		td, err := queries.GetTokenDefinitionByTokenDbid(ctx, n.Data.NewTokenID)
		if err != nil {
			return UserFacingNotificationData{}, err
		}

		nameFound := td.Name.String != ""

		if !nameFound {
			// name does not exist yet, the token might still be refreshing so let's try and wait a little bit and retry a few times
			<-time.After(5 * time.Second)
			retry.RetryFunc(ctx, func(ctx context.Context) error {
				td, err = queries.GetTokenDefinitionByTokenDbid(ctx, n.Data.NewTokenID)
				if err != nil {
					return err
				}
				if td.Name.String == "" {
					return errNameNotAvailable
				}
				nameFound = true
				return nil
			}, func(err error) bool {
				return err == errNameNotAvailable
			}, retry.Retry{MinWait: 5, MaxWait: 30, MaxRetries: 3})
		}

		name := util.TruncateWithEllipsis(td.Name.String, 40)

		if !nameFound {
			data.Actor = "You"
			data.Action = "just collected an NFT. Tap to share now."
			return data, nil
		}

		data.Actor = "You"
		data.Action = fmt.Sprintf("just collected %s. Tap to share now.", name)
		return data, nil
	case persist.ActionUserPostedYourWork:
		post, err := queries.GetPostByID(ctx, n.PostID)
		if err != nil {
			return UserFacingNotificationData{}, err
		}
		actor, err := queries.GetUserById(ctx, post.ActorID)
		if err != nil {
			return UserFacingNotificationData{}, err
		}
		if !actor.Username.Valid {
			return UserFacingNotificationData{}, fmt.Errorf("user with ID=%s has no username", actor.ID)
		}
		community, err := queries.GetCommunityByID(ctx, n.CommunityID)
		if err != nil {
			return UserFacingNotificationData{}, err
		}
		return UserFacingNotificationData{
			Actor:          actor.Username.String,
			Action:         "posted your work",
			CollectionName: community.Name,
			CollectionID:   community.ID,
			PreviewText:    util.TruncateWithEllipsis(post.Caption.String, 40),
		}, nil

	case persist.ActionUserPostedFirstPost:
		post, err := queries.GetPostByID(ctx, n.PostID)
		if err != nil {
			return UserFacingNotificationData{}, err
		}
		actor, err := queries.GetUserById(ctx, post.ActorID)
		if err != nil {
			return UserFacingNotificationData{}, err
		}
		if !actor.Username.Valid {
			return UserFacingNotificationData{}, fmt.Errorf("user with ID=%s has no username", actor.ID)
		}
		return UserFacingNotificationData{
			Actor:       actor.Username.String,
			Action:      "made their first post",
			PreviewText: util.TruncateWithEllipsis(post.Caption.String, 40),
		}, nil
	case persist.ActionTopActivityBadgeReceived:
		return UserFacingNotificationData{
			Actor:  "You",
			Action: "received a new badge for being amongst the top active users on Gallery this week!",
		}, nil
	case persist.ActionAnnouncement:
		return UserFacingNotificationData{
			Action: n.Data.AnnouncementDetails.PushNotificationText,
		}, nil
	case persist.ActionUserFromFarcasterJoined:
		actor, err := queries.GetUserById(ctx, n.Data.UserFromFarcasterJoinedDetails.UserID)
		if err != nil {
			return UserFacingNotificationData{}, err
		}
		if !actor.Username.Valid {
			return UserFacingNotificationData{}, fmt.Errorf("user with ID=%s has no username", actor.ID)
		}
		return UserFacingNotificationData{
			Actor:  actor.Username.String,
			Action: "from Farcaster is now on Gallery",
		}, nil
	default:
		return UserFacingNotificationData{}, fmt.Errorf("unknown action %s", n.Action)
	}
}

func actionSupportsPushNotifications(action persist.Action) bool {
	switch action {
	case persist.ActionAdmiredFeedEvent:
		return true
	case persist.ActionCommentedOnFeedEvent:
		return true
	case persist.ActionUserFollowedUsers:
		return true
	case persist.ActionNewTokensReceived:
		return true
	case persist.ActionCommentedOnPost:
		return true
	case persist.ActionAdmiredPost:
		return true
	case persist.ActionAdmiredToken:
		return true
	case persist.ActionAdmiredComment:
		return true
	case persist.ActionUserPostedYourWork:
		return true
	case persist.ActionMentionUser:
		return true
	case persist.ActionMentionCommunity:
		return true
	case persist.ActionUserPostedFirstPost:
		return true
	case persist.ActionTopActivityBadgeReceived:
		return false // TODO -activity
	case persist.ActionAnnouncement:
		return true
	case persist.ActionUserFromFarcasterJoined:
		return true
	default:
		return false
	}
}

func sendPushNotifications(ctx context.Context, notif db.Notification, queries *db.Queries, taskClient *task.Client, limiter *pushLimiter) error {
	if !actionSupportsPushNotifications(notif.Action) {
		return nil
	}

	pushTokens, err := queries.GetPushTokensByUserID(ctx, notif.OwnerID)
	if err != nil {
		return fmt.Errorf("couldn't get push tokens for userID %s: %w", notif.OwnerID, err)
	}

	// Don't try to send anything if the user doesn't have any registered push tokens
	if len(pushTokens) == 0 {
		return nil
	}

	message, err := createPushMessage(ctx, notif, queries, limiter)
	if err != nil {
		if util.ErrorIs[errRateLimited](err) {
			// Rate limiting is expected and shouldn't be propagated upward as an error
			logger.For(ctx).Infof("couldn't create push message: %s", err)
			return nil
		} else if errors.Is(err, errAccountTooNew) {
			// Some message types (e.g. follow notifications) shouldn't be triggered by new accounts
			logger.For(ctx).Infof("not sending push notification to %s because sending user is too new", notif.OwnerID)
			return nil
		}

		return fmt.Errorf("couldn't create push message: %w", err)
	}

	for _, token := range pushTokens {
		toSend := message
		toSend.PushTokenID = token.ID
		err = taskClient.CreateTaskForPushNotification(ctx, toSend)
		if err != nil {
			err = fmt.Errorf("failed to create task for push notification: %w", err)
			sentryutil.ReportError(ctx, err)
			logger.For(ctx).Error(err)
		}
	}

	return nil
}

func insertAndPublishNotif(ctx context.Context, notif db.Notification, queries *db.Queries, ps *pubsub.Client, taskClient *task.Client, limiter *pushLimiter) error {
	newNotif, err := addNotification(ctx, notif, queries)
	if err != nil {
		return fmt.Errorf("failed to create notification: %w", err)
	}

	err = sendNotification(ctx, newNotif, queries, taskClient, limiter, ps)
	if err != nil {
		return err
	}

	logger.For(ctx).Infof("pushed new notification to pubsub: %s", notif.OwnerID)

	return nil
}

func insertAndPublishAnnouncementNotifs(ctx context.Context, notif db.Notification, queries *db.Queries, ps *pubsub.Client, taskClient *task.Client, limiter *pushLimiter) error {
	notifs, err := addGlobalNotifications(ctx, notif, queries)
	if err != nil {
		return fmt.Errorf("failed to create notification: %w", err)
	}

	if (notif.Data.AnnouncementDetails.Platform == persist.AnnouncementPlatformAll || notif.Data.AnnouncementDetails.Platform == persist.AnnouncementPlatformMobile) && notif.Data.AnnouncementDetails.PushNotificationText != "" {
		logger.For(ctx).Infof("sending push notifications for announcement: %s", notif.Data.AnnouncementDetails.InternalID)
		p := pool.New().WithErrors().WithContext(ctx).WithMaxGoroutines(10)
		for _, notif := range notifs {
			notif := notif
			p.Go(func(ctx context.Context) error {
				return sendNotification(ctx, notif, queries, taskClient, limiter, ps)
			})
		}
	}

	logger.For(ctx).Infof("pushed new notification to pubsub: %s", notif.OwnerID)

	return nil
}

// this function only sends push notifications, could probaby be renamed
func sendNotification(ctx context.Context, newNotif db.Notification, queries *db.Queries, taskClient *task.Client, limiter *pushLimiter, ps *pubsub.Client) error {
	err := sendPushNotifications(ctx, newNotif, queries, taskClient, limiter)
	if err != nil {
		err = fmt.Errorf("failed to send push notifications for notification with DBID=%s, error: %w", newNotif.ID, err)
		sentryutil.ReportError(ctx, err)
		logger.For(ctx).Error(err)
	}

	marshalled, err := json.Marshal(newNotif)
	if err != nil {
		return err
	}
	t := ps.Topic(env.GetString("PUBSUB_TOPIC_NEW_NOTIFICATIONS"))
	result := t.Publish(ctx, &pubsub.Message{
		Data: marshalled,
	})

	_, err = result.Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to publish new notification: %w", err)
	}
	return nil
}

func updateAndPublishNotif(ctx context.Context, notif db.Notification, mostRecentNotif db.Notification, queries *db.Queries, ps *pubsub.Client, taskClient *task.Client, limiter *pushLimiter) error {
	var amount = notif.Amount
	resultData := mostRecentNotif.Data.Concat(notif.Data)
	switch notif.Action {
	case persist.ActionAdmiredFeedEvent, persist.ActionAdmiredPost, persist.ActionAdmiredToken, persist.ActionAdmiredComment:
		amount = int32(len(resultData.AdmirerIDs))
	case persist.ActionViewedGallery:
		amount = int32(len(resultData.AuthedViewerIDs) + len(resultData.UnauthedViewerIDs))
	case persist.ActionUserFollowedUsers:
		amount = int32(len(resultData.FollowerIDs))
	case persist.ActionNewTokensReceived:
		amount = int32(resultData.NewTokenQuantity.BigInt().Uint64())
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

	err = sendPushNotifications(ctx, notif, queries, taskClient, limiter)
	if err != nil {
		err = fmt.Errorf("failed to send push notifications for notification with DBID=%s, error: %w", notif.ID, err)
		sentryutil.ReportError(ctx, err)
		logger.For(ctx).Error(err)
	}

	updatedNotif, err := queries.GetNotificationByID(ctx, mostRecentNotif.ID)
	if err != nil {
		return fmt.Errorf("error getting updated notification by %s: %w", mostRecentNotif.ID, err)
	}
	marshalled, err := json.Marshal(updatedNotif)
	if err != nil {
		return err
	}
	t := ps.Topic(env.GetString("PUBSUB_TOPIC_UPDATED_NOTIFICATIONS"))
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
			ID:        id,
			OwnerID:   notif.OwnerID,
			Action:    notif.Action,
			Data:      notif.Data,
			EventIds:  notif.EventIds,
			FeedEvent: sql.NullString{String: notif.FeedEventID.String(), Valid: true},
		})
	case persist.ActionCommentedOnFeedEvent:
		return queries.CreateCommentNotification(ctx, db.CreateCommentNotificationParams{
			ID:        id,
			OwnerID:   notif.OwnerID,
			Action:    notif.Action,
			Data:      notif.Data,
			EventIds:  notif.EventIds,
			FeedEvent: sql.NullString{String: notif.FeedEventID.String(), Valid: true},
			CommentID: notif.CommentID,
		})
	case persist.ActionAdmiredPost:
		return queries.CreateAdmireNotification(ctx, db.CreateAdmireNotificationParams{
			ID:       id,
			OwnerID:  notif.OwnerID,
			Action:   notif.Action,
			Data:     notif.Data,
			EventIds: notif.EventIds,
			Post:     sql.NullString{String: notif.PostID.String(), Valid: true},
		})
	case persist.ActionAdmiredToken:
		return queries.CreateAdmireNotification(ctx, db.CreateAdmireNotificationParams{
			ID:       id,
			OwnerID:  notif.OwnerID,
			Action:   notif.Action,
			Data:     notif.Data,
			EventIds: notif.EventIds,
			Token:    sql.NullString{String: notif.TokenID.String(), Valid: true},
		})
	case persist.ActionAdmiredComment:
		return queries.CreateCommentNotification(ctx, db.CreateCommentNotificationParams{
			ID:        id,
			OwnerID:   notif.OwnerID,
			Action:    notif.Action,
			Data:      notif.Data,
			EventIds:  notif.EventIds,
			CommentID: notif.CommentID,
		})
	case persist.ActionCommentedOnPost:
		return queries.CreateCommentNotification(ctx, db.CreateCommentNotificationParams{
			ID:        id,
			OwnerID:   notif.OwnerID,
			Action:    notif.Action,
			Data:      notif.Data,
			EventIds:  notif.EventIds,
			Post:      sql.NullString{String: notif.PostID.String(), Valid: true},
			CommentID: notif.CommentID,
		})
	case persist.ActionUserFollowedUsers, persist.ActionTopActivityBadgeReceived:
		return queries.CreateSimpleNotification(ctx, db.CreateSimpleNotificationParams{
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
	case persist.ActionNewTokensReceived:
		amount := notif.Data.NewTokenQuantity.BigInt().Int64()
		return queries.CreateTokenNotification(ctx, db.CreateTokenNotificationParams{
			ID:       id,
			OwnerID:  notif.OwnerID,
			Action:   notif.Action,
			Data:     notif.Data,
			EventIds: notif.EventIds,
			TokenID:  notif.TokenID,
			Amount:   int32(amount),
		})
	case persist.ActionReplyToComment:
		return queries.CreateCommentNotification(ctx, db.CreateCommentNotificationParams{
			ID:        id,
			OwnerID:   notif.OwnerID,
			Action:    notif.Action,
			Data:      notif.Data,
			EventIds:  notif.EventIds,
			CommentID: notif.CommentID,
			FeedEvent: util.ToNullString(notif.FeedEventID.String(), true),
			Post:      util.ToNullString(notif.PostID.String(), true),
		})
	case persist.ActionMentionUser:
		return queries.CreateMentionUserNotification(ctx, db.CreateMentionUserNotificationParams{
			ID:        id,
			OwnerID:   notif.OwnerID,
			Action:    notif.Action,
			Data:      notif.Data,
			EventIds:  notif.EventIds,
			FeedEvent: util.ToNullString(notif.FeedEventID.String(), true),
			Post:      util.ToNullString(notif.PostID.String(), true),
			Comment:   util.ToNullString(notif.CommentID.String(), true),
			MentionID: notif.MentionID,
		})
	case persist.ActionMentionCommunity:
		return queries.CreateCommunityNotification(ctx, db.CreateCommunityNotificationParams{
			ID:          id,
			OwnerID:     notif.OwnerID,
			Action:      notif.Action,
			Data:        notif.Data,
			EventIds:    notif.EventIds,
			FeedEvent:   util.ToNullString(notif.FeedEventID.String(), true),
			Post:        util.ToNullString(notif.PostID.String(), true),
			Comment:     util.ToNullString(notif.CommentID.String(), true),
			MentionID:   notif.MentionID,
			CommunityID: notif.CommunityID,
		})
	case persist.ActionUserPostedYourWork:
		return queries.CreateUserPostedYourWorkNotification(ctx, db.CreateUserPostedYourWorkNotificationParams{
			ID:          id,
			OwnerID:     notif.OwnerID,
			Action:      notif.Action,
			Data:        notif.Data,
			EventIds:    notif.EventIds,
			Post:        util.ToNullString(notif.PostID.String(), true),
			CommunityID: notif.CommunityID,
		})
	case persist.ActionUserFromFarcasterJoined:
		return queries.CreateSimpleNotification(ctx, db.CreateSimpleNotificationParams{
			ID:       id,
			OwnerID:  notif.OwnerID,
			Action:   notif.Action,
			Data:     notif.Data,
			EventIds: notif.EventIds,
		})
	case persist.ActionUserPostedFirstPost:
		return queries.CreatePostNotification(ctx, db.CreatePostNotificationParams{
			ID:       id,
			OwnerID:  notif.OwnerID,
			Action:   notif.Action,
			Data:     notif.Data,
			EventIds: notif.EventIds,
			PostID:   notif.PostID,
		})
	default:
		return db.Notification{}, fmt.Errorf("unknown notification action: %s", notif.Action)
	}
}

func addGlobalNotifications(ctx context.Context, notif db.Notification, queries *db.Queries) ([]db.Notification, error) {

	userCount, err := queries.CountAllUsers(ctx)
	if err != nil {
		return nil, err
	}

	ids := make([]string, userCount)
	for i := 0; i < int(userCount); i++ {
		ids[i] = persist.GenerateID().String()
	}

	notifs, err := queries.CreateAnnouncementNotifications(ctx, db.CreateAnnouncementNotificationsParams{
		Ids:      ids,
		Action:   notif.Action,
		Data:     notif.Data,
		EventIds: notif.EventIds,
		Internal: notif.Data.AnnouncementDetails.InternalID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create follower notifications: %w", err)
	}

	return notifs, nil
}

func (l lockKey) String() string {
	return fmt.Sprintf("%s:%s", l.ownerID, l.action)
}

func errTopicMissing(err error) bool {
	var aErr *apierror.APIError
	if ok := errors.As(err, &aErr); ok && aErr.GRPCStatus().Code() == codes.NotFound {
		return true
	}
	return false
}

func createSubscription(ctx context.Context, client *pubsub.Client, topic, name string) (*pubsub.Subscription, error) {
	return client.CreateSubscription(ctx, name, pubsub.SubscriptionConfig{
		Topic:            client.Topic(topic),
		ExpirationPolicy: time.Hour * 24 * 3,
	})
}

var errAccountTooNew = errors.New("not sending a push notification: user was created less than 24 hours ago")

type errRateLimited struct {
	limiterName string
	senderID    persist.DBID
	receiverID  persist.DBID
	feedEventID persist.DBID
	tokenID     persist.DBID
	commentID   persist.DBID
}

func (e errRateLimited) Error() string {
	str := fmt.Sprintf("rate limit exceeded for limiter=%s, sender=%s, receiver=%s", e.limiterName, e.senderID, e.receiverID)
	if e.feedEventID != "" {
		str += fmt.Sprintf(", feedEvent=%s", e.feedEventID)
	}
	if e.commentID != "" {
		str += fmt.Sprintf(", commentID=%s", e.commentID)
	}
	return str
}
