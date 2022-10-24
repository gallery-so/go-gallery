package event

import (
	"context"
	"fmt"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/gin-gonic/gin"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
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

const EventHandlerContextKey = "event.eventHandlers"

type EventHandlers struct {
	EventDispatcher      *eventDispatcher
	notificationHandlers *notifications.NotificationHandlers
}

// Register specific event handlers
func AddTo(ctx *gin.Context, disableDataloaderCaching bool, notif *notifications.NotificationHandlers, queries *db.Queries, taskClient *cloudtasks.Client) {
	eventRepo := postgres.EventRepository{Queries: queries}
	eventDispatcher := eventDispatcher{eventRepo: eventRepo, handlers: map[persist.Action][]eventHandler{}}
	feedHandler := feedHandler{tc: taskClient}
	notificationHandler := notificationHandler{notificationHandlers: notif, dataloaders: dataloader.NewLoaders(context.Background(), queries, disableDataloaderCaching)}

	eventDispatcher.AddHandler(persist.ActionUserCreated, feedHandler)
	eventDispatcher.AddHandler(persist.ActionUserFollowedUsers, feedHandler, notificationHandler)
	eventDispatcher.AddHandler(persist.ActionCollectorsNoteAddedToToken, feedHandler)
	eventDispatcher.AddHandler(persist.ActionCollectionCreated, feedHandler)
	eventDispatcher.AddHandler(persist.ActionCollectorsNoteAddedToCollection, feedHandler)
	eventDispatcher.AddHandler(persist.ActionTokensAddedToCollection, feedHandler)
	eventDispatcher.AddHandler(persist.ActionAdmiredFeedEvent, notificationHandler)
	eventDispatcher.AddHandler(persist.ActionViewedGallery, notificationHandler)
	eventDispatcher.AddHandler(persist.ActionCommentedOnFeedEvent, notificationHandler)

	eventHandlers := &EventHandlers{EventDispatcher: &eventDispatcher, notificationHandlers: notif}
	ctx.Set(EventHandlerContextKey, eventHandlers)
}

func DispatchEvent(ctx context.Context, event db.Event) error {
	gc := util.GinContextFromContext(ctx)
	return For(gc).EventDispatcher.Dispatch(ctx, event)
}

func For(ctx context.Context) *EventHandlers {
	gc := util.GinContextFromContext(ctx)
	return gc.Value(EventHandlerContextKey).(*EventHandlers)
}

type eventHandler interface {
	Handle(context.Context, db.Event) error
}

type eventDispatcher struct {
	eventRepo postgres.EventRepository
	handlers  map[persist.Action][]eventHandler
}

func (d *eventDispatcher) AddHandler(action persist.Action, handlers ...eventHandler) {
	d.handlers[action] = append(d.handlers[action], handlers...)
}

func (d *eventDispatcher) Dispatch(ctx context.Context, event db.Event) error {
	if handlers, ok := d.handlers[event.Action]; ok {
		eg, newCtx := errgroup.WithContext(ctx)
		persisted, err := d.eventRepo.Add(newCtx, event)
		if err != nil {
			return err
		}
		for _, handler := range handlers {
			h := handler
			eg.Go(func() error {
				return h.Handle(newCtx, *persisted)
			})
		}
		return eg.Wait()
	}
	logger.For(ctx).Warnf("no handler registered for action: %s", event.Action)
	return nil
}

type feedHandler struct {
	tc *cloudtasks.Client
}

func (h feedHandler) Handle(ctx context.Context, persistedEvent db.Event) error {
	scheduleOn := persistedEvent.CreatedAt.Add(time.Duration(viper.GetInt("GCLOUD_FEED_BUFFER_SECS")) * time.Second)
	return task.CreateTaskForFeed(ctx, scheduleOn, task.FeedMessage{ID: persistedEvent.ID}, h.tc)
}

type notificationHandler struct {
	queries              *db.Queries
	dataloaders          *dataloader.Loaders
	notificationHandlers *notifications.NotificationHandlers
}

func (h notificationHandler) Handle(ctx context.Context, persistedEvent db.Event) error {

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
