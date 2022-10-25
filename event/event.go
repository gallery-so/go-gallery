package event

import (
	"context"
	"fmt"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/gin-gonic/gin"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/feed"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/notifications"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
)

type sendType int

const (
	eventSenderContextKey          = "event.eventSender"
	delayedKey            sendType = iota
	immediateKey
)

// Register specific event handlers
func AddTo(ctx *gin.Context, disableDataloaderCaching bool, notif *notifications.NotificationHandlers, queries *db.Queries, taskClient *cloudtasks.Client) {
	sender := eventSender{
		registry:  map[sendType]registedActions{delayedKey: {}, immediateKey: {}},
		eventRepo: postgres.EventRepository{Queries: queries},
	}

	feed := newEventDispatcher("feed")
	feedHandler := newFeedHandler(queries, taskClient)
	sender.addDelayedHandler(feed, persist.ActionUserCreated, feedHandler)
	sender.addDelayedHandler(feed, persist.ActionUserFollowedUsers, feedHandler)
	sender.addDelayedHandler(feed, persist.ActionCollectorsNoteAddedToToken, feedHandler)
	sender.addDelayedHandler(feed, persist.ActionCollectionCreated, feedHandler)
	sender.addDelayedHandler(feed, persist.ActionCollectorsNoteAddedToToken, feedHandler)
	sender.addDelayedHandler(feed, persist.ActionTokensAddedToCollection, feedHandler)
	sender.addImmediateHandler(feed, persist.ActionCollectionCreated, feedHandler)
	sender.addImmediateHandler(feed, persist.ActionTokensAddedToCollection, feedHandler)

	notifications := newEventDispatcher("notifications")
	notificationHandler := newNotificationHandler(notif, disableDataloaderCaching, queries)
	sender.addDelayedHandler(notifications, persist.ActionUserFollowedUsers, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionAdmiredFeedEvent, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionViewedGallery, notificationHandler)
	sender.addDelayedHandler(notifications, persist.ActionCommentedOnFeedEvent, notificationHandler)

	sender.feed = feed
	sender.notifications = notifications
	ctx.Set(eventSenderContextKey, &sender)
}

// DispatchEvent sends the event to all of its registered handlers.
func DispatchDelayed(ctx context.Context, event db.Event) error {
	gc := util.GinContextFromContext(ctx)
	handlers := For(gc)

	if _, handable := handlers.registry[delayedKey][event.Action]; !handable {
		logger.For(ctx).Warnf("no handler configured for action: %s", event.Action)
		return nil
	}

	persistedEvent, err := handlers.eventRepo.Add(ctx, event)
	if err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error { return handlers.feed.dispatchDelayed(ctx, *persistedEvent) })
	eg.Go(func() error { return handlers.notifications.dispatchDelayed(ctx, *persistedEvent) })
	return eg.Wait()
}

func DispatchImmediate(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	gc := util.GinContextFromContext(ctx)
	handlers := For(gc)

	if _, handable := handlers.registry[immediateKey][event.Action]; !handable {
		logger.For(ctx).Warnf("no handler configured for action: %s", event.Action)
		return nil, nil
	}

	persistedEvent, err := handlers.eventRepo.Add(ctx, event)
	if err != nil {
		return nil, err
	}

	feedEvent, err := handlers.feed.dispatchImmediate(ctx, *persistedEvent)
	if err != nil {
		return nil, err
	}
	err = handlers.notifications.dispatchDelayed(ctx, *persistedEvent)
	if err != nil {
		return nil, err
	}

	return feedEvent.(*db.FeedEvent), nil
}

func For(ctx context.Context) *eventSender {
	gc := util.GinContextFromContext(ctx)
	return gc.Value(eventSenderContextKey).(*eventSender)
}

type registedActions map[persist.Action]struct{}

type eventSender struct {
	feed          *eventDispatcher
	notifications *eventDispatcher
	registry      map[sendType]registedActions
	eventRepo     postgres.EventRepository
}

func (e *eventSender) addDelayedHandler(dispatcher *eventDispatcher, action persist.Action, handler delayedHandler) {
	dispatcher.addDelayed(action, handler)
	e.registry[delayedKey][action] = struct{}{}
}

func (e *eventSender) addImmediateHandler(dispatcher *eventDispatcher, action persist.Action, handler immediateHandler) {
	dispatcher.addImmediate(action, handler)
	e.registry[immediateKey][action] = struct{}{}
}

type eventDispatcher struct {
	service           string
	delayedHandlers   map[persist.Action]delayedHandler
	immediateHandlers map[persist.Action]immediateHandler
}

func newEventDispatcher(service string) *eventDispatcher {
	return &eventDispatcher{
		service:           service,
		delayedHandlers:   map[persist.Action]delayedHandler{},
		immediateHandlers: map[persist.Action]immediateHandler{},
	}
}

func (d *eventDispatcher) addDelayed(action persist.Action, handler delayedHandler) {
	d.delayedHandlers[action] = handler
}

func (d *eventDispatcher) addImmediate(action persist.Action, handler immediateHandler) {
	d.immediateHandlers[action] = handler
}

func (d *eventDispatcher) dispatchDelayed(ctx context.Context, event db.Event) error {
	if handler, ok := d.delayedHandlers[event.Action]; ok {
		return handler.handleDelayed(ctx, event)
	}
	logger.For(ctx).WithField("service", d.service).Warnf("no delayed handler registered for action: %s", event.Action)
	return nil
}

func (d *eventDispatcher) dispatchImmediate(ctx context.Context, event db.Event) (interface{}, error) {
	if handler, ok := d.immediateHandlers[event.Action]; ok {
		return handler.handleImmediate(ctx, event)
	}
	logger.For(ctx).WithField("service", d.service).Warnf("no immediate handler registered for action: %s", event.Action)
	return nil, nil
}

type delayedHandler interface {
	handleDelayed(context.Context, db.Event) error
}

type immediateHandler interface {
	handleImmediate(context.Context, db.Event) (interface{}, error)
}

// feedHandler handles events for consumption as feed events.
type feedHandler struct {
	eventBuilder *feed.EventBuilder
	tc           *cloudtasks.Client
}

func newFeedHandler(queries *db.Queries, taskClient *cloudtasks.Client) feedHandler {
	return feedHandler{
		eventBuilder: feed.NewEventBuilder(queries, true),
		tc:           taskClient,
	}
}

// handleDelayed creates a delayed task for the Feed service to handle later.
func (h feedHandler) handleDelayed(ctx context.Context, persistedEvent db.Event) error {
	scheduleOn := persistedEvent.CreatedAt.Add(time.Duration(viper.GetInt("GCLOUD_FEED_BUFFER_SECS")) * time.Second)
	return task.CreateTaskForFeed(ctx, scheduleOn, task.FeedMessage{ID: persistedEvent.ID}, h.tc)
}

// handledImmediate sidesteps the Feed service so that an event is immediately available as a feed event.
func (h feedHandler) handleImmediate(ctx context.Context, persistedEvent db.Event) (interface{}, error) {
	return h.eventBuilder.NewEvent(ctx, persistedEvent)
}

// notificationHandlers handles events for consumption as notifications.
type notificationHandler struct {
	dataloaders          *dataloader.Loaders
	notificationHandlers *notifications.NotificationHandlers
}

func newNotificationHandler(notifiers *notifications.NotificationHandlers, disableDataloaderCaching bool, queries *db.Queries) *notificationHandler {
	return &notificationHandler{
		notificationHandlers: notifiers,
		dataloaders:          dataloader.NewLoaders(context.Background(), queries, disableDataloaderCaching),
	}
}

func (h notificationHandler) handleDelayed(ctx context.Context, persistedEvent db.Event) error {
	owner, err := h.findOwnerForNotificationFromEvent(persistedEvent)
	if err != nil {
		return err
	}
	return h.notificationHandlers.Notifications.Dispatch(ctx, db.Notification{
		OwnerID:     owner,
		Action:      persistedEvent.Action,
		Data:        h.createNotificationDataForEvent(persistedEvent),
		EventIds:    persist.DBIDList{persistedEvent.ID},
		GalleryID:   persistedEvent.GalleryID,
		FeedEventID: persistedEvent.FeedEventID,
		CommentID:   persistedEvent.CommentID,
	})
}

func (h notificationHandler) createNotificationDataForEvent(event db.Event) (data persist.NotificationData) {
	switch event.Action {
	case persist.ActionViewedGallery:
		if event.ActorID != "" {
			data.AuthedViewerIDs = []persist.DBID{event.ActorID}
		}
		if event.ExternalID != "" {
			data.UnauthedViewerIDs = []persist.NullString{event.ExternalID}
		}
	case persist.ActionAdmiredFeedEvent:
		if event.ActorID != "" {
			data.AdmirerIDs = []persist.DBID{event.ActorID}
		}
	case persist.ActionUserFollowedUsers:
		if event.ActorID != "" {
			data.FollowerIDs = []persist.DBID{event.ActorID}
		}
		data.FollowedBack = persist.NullBool(event.Data.UserFollowedBack)
		data.Refollowed = persist.NullBool(event.Data.UserRefollowed)
	}
	return
}

func (h notificationHandler) findOwnerForNotificationFromEvent(event db.Event) (persist.DBID, error) {
	switch event.ResourceTypeID {
	case persist.ResourceTypeGallery:
		gallery, err := h.dataloaders.GalleryByGalleryID.Load(event.GalleryID)
		if err != nil {
			return "", err
		}
		return gallery.OwnerUserID, nil
	case persist.ResourceTypeFeedEvent:
		feedEvent, err := h.dataloaders.FeedEventByFeedEventID.Load(event.FeedEventID)
		if err != nil {
			return "", err
		}
		return feedEvent.OwnerID, nil
	case persist.ResourceTypeUser:
		return event.UserID, nil
	}

	return "", fmt.Errorf("no owner found for event: %s", event.Action)
}
