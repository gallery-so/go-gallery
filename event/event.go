package event

import (
	"context"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/gin-gonic/gin"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/feed"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

const EventHandlerContextKey = "event.eventHandlers"

type EventHandlers struct {
	Feed *eventDispatcher
}

// Register specific event handlers
func AddTo(ctx *gin.Context, queries *db.Queries, taskClient *cloudtasks.Client) {
	eventDispatcher := newEventDispatcher()
	feedHandler := newFeedHandler(queries, taskClient)

	eventDispatcher.AddHandler(persist.ActionUserCreated, feedHandler)
	eventDispatcher.AddHandler(persist.ActionUserFollowedUsers, feedHandler)
	eventDispatcher.AddHandler(persist.ActionCollectorsNoteAddedToToken, feedHandler)
	eventDispatcher.AddHandler(persist.ActionCollectionCreated, feedHandler)
	eventDispatcher.AddHandler(persist.ActionCollectorsNoteAddedToCollection, feedHandler)
	eventDispatcher.AddHandler(persist.ActionTokensAddedToCollection, feedHandler)

	eventDispatcher.AddImmediateHandler(persist.ActionCollectionCreated, feedHandler)
	eventDispatcher.AddImmediateHandler(persist.ActionTokensAddedToCollection, feedHandler)

	eventHandlers := &EventHandlers{Feed: &eventDispatcher}
	ctx.Set(EventHandlerContextKey, eventHandlers)
}

func DispatchEventToFeed(ctx context.Context, event db.Event) error {
	gc := util.GinContextFromContext(ctx)
	return For(gc).Feed.Dispatch(ctx, event)
}

func SaveImmediateToFeed(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	gc := util.GinContextFromContext(ctx)
	feedEvent, err := For(gc).Feed.InvokeHandler(ctx, event)
	if err != nil {
		return nil, err
	}
	return feedEvent.(*db.FeedEvent), nil
}

func For(ctx context.Context) *EventHandlers {
	gc := util.GinContextFromContext(ctx)
	return gc.Value(EventHandlerContextKey).(*EventHandlers)
}

type eventDispatcher struct {
	handlers          map[persist.Action]eventHandler
	immediateHandlers map[persist.Action]immediateHandler
}

func newEventDispatcher() eventDispatcher {
	return eventDispatcher{
		handlers:          map[persist.Action]eventHandler{},
		immediateHandlers: map[persist.Action]immediateHandler{},
	}
}

func (d *eventDispatcher) AddHandler(action persist.Action, handler eventHandler) {
	d.handlers[action] = handler
}

func (d *eventDispatcher) AddImmediateHandler(action persist.Action, handler immediateHandler) {
	d.immediateHandlers[action] = handler
}

func (d *eventDispatcher) Dispatch(ctx context.Context, event db.Event) error {
	if handler, ok := d.handlers[event.Action]; ok {
		return handler.Handle(ctx, event)
	}
	logger.For(ctx).Warnf("no handler registered for action: %s", event.Action)
	return nil
}

func (d *eventDispatcher) InvokeHandler(ctx context.Context, event db.Event) (interface{}, error) {
	if handler, ok := d.immediateHandlers[event.Action]; ok {
		return handler.Invoke(ctx, event)
	}
	logger.For(ctx).Warnf("no handler registered for action: %s", event.Action)
	return nil, nil
}

type eventHandler interface {
	Handle(context.Context, db.Event) error
}

type immediateHandler interface {
	Invoke(context.Context, db.Event) (interface{}, error)
}

type feedHandler struct {
	eventRepo    postgres.EventRepository
	eventBuilder *feed.EventBuilder
	tc           *cloudtasks.Client
}

func newFeedHandler(queries *db.Queries, taskClient *cloudtasks.Client) feedHandler {
	handler := feedHandler{
		eventRepo:    postgres.EventRepository{Queries: queries},
		eventBuilder: feed.NewEventBuilder(queries, true),
		tc:           taskClient,
	}
	return handler
}

// Handle creates a delayed task for the Feed service to handle later.
func (h feedHandler) Handle(ctx context.Context, event db.Event) error {
	persisted, err := h.eventRepo.Add(ctx, event)
	if err != nil {
		return err
	}

	scheduleOn := persisted.CreatedAt.Add(time.Duration(viper.GetInt("GCLOUD_FEED_BUFFER_SECS")) * time.Second)
	return task.CreateTaskForFeed(ctx, scheduleOn, task.FeedMessage{ID: persisted.ID}, h.tc)
}

// Invoke sidesteps the Feed service so that an event is immediately available as a feed event.
func (h feedHandler) Invoke(ctx context.Context, event db.Event) (interface{}, error) {
	persisted, err := h.eventRepo.Add(ctx, event)
	if err != nil {
		return nil, err
	}
	return h.eventBuilder.NewEvent(ctx, *persisted)
}
