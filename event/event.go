package event

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

const EventHandlerContextKey = "event.eventHandlers"

type EventHandlers struct {
	Feed *eventDispatcher

	// TODO: Remove when the feedbot uses the feed API instead of creating its own posts.
	Collection *CollectionDispatcher
	User       *UserDispatcher
	Nft        *NftDispatcher
}

// Register specific event handlers
func AddTo(ctx *gin.Context, repos *persist.Repositories, queries *sqlc.Queries) {
	eventDispatcher := eventDispatcher{handlers: map[persist.Action]eventHandler{}}
	feedHandler := feedHandler{postgres.EventRepository{Queries: queries}}

	eventDispatcher.AddHandler(persist.ActionUserCreated, feedHandler)
	eventDispatcher.AddHandler(persist.ActionUserFollowedUsers, feedHandler)
	eventDispatcher.AddHandler(persist.ActionCollectorsNoteAddedToToken, feedHandler)
	eventDispatcher.AddHandler(persist.ActionCollectionCreated, feedHandler)
	eventDispatcher.AddHandler(persist.ActionCollectorsNoteAddedToCollection, feedHandler)
	eventDispatcher.AddHandler(persist.ActionTokensAddedToCollection, feedHandler)

	// TODO: Remove when the feedbot uses the feed API instead of creating its own posts.
	collectionDispatcher := CollectionDispatcher{Handlers: map[persist.EventCode][]CollectionEventHandler{}}
	collectionTask := task.CollectionFeedTask{CollectionEventRepo: repos.CollectionEventRepository}
	collectionDispatcher.Handle(persist.CollectionCreatedEvent, &collectionTask)
	collectionDispatcher.Handle(persist.CollectionCollectorsNoteAdded, &collectionTask)
	collectionDispatcher.Handle(persist.CollectionTokensAdded, &collectionTask)
	userDispatcher := UserDispatcher{Handlers: map[persist.EventCode][]UserEventHandler{}}
	userTask := task.UserFeedTask{UserEventRepo: repos.UserEventRepository}
	userDispatcher.Handle(persist.UserCreatedEvent, &userTask)
	userDispatcher.Handle(persist.UserFollowedEvent, &userTask)
	nftDispatcher := NftDispatcher{Handlers: map[persist.EventCode][]NftEventHandler{}}
	nftTask := task.NftFeedEvent{NftEventRepo: repos.NftEventRepository}
	nftDispatcher.Handle(persist.NftCollectorsNoteAddedEvent, &nftTask)

	eventHandlers := &EventHandlers{
		Feed: &eventDispatcher,

		// TODO: Remove when the feedbot uses the feed API instead of creating its own posts.
		Collection: &collectionDispatcher,
		User:       &userDispatcher,
		Nft:        &nftDispatcher,
	}

	ctx.Set(EventHandlerContextKey, eventHandlers)
}

func DispatchEventToFeed(ctx context.Context, event sqlc.Event) error {
	gc := util.GinContextFromContext(ctx)
	return For(gc).Feed.Dispatch(ctx, event)
}

func For(ctx context.Context) *EventHandlers {
	gc := util.GinContextFromContext(ctx)
	return gc.Value(EventHandlerContextKey).(*EventHandlers)
}

type eventHandler interface {
	Handle(context.Context, sqlc.Event) error
}

type eventDispatcher struct {
	handlers map[persist.Action]eventHandler
}

func (d *eventDispatcher) AddHandler(action persist.Action, handler eventHandler) {
	d.handlers[action] = handler
}

func (d *eventDispatcher) Dispatch(ctx context.Context, event sqlc.Event) error {
	if handler, ok := d.handlers[event.Action]; ok {
		return handler.Handle(ctx, event)
	}
	return nil
}

type feedHandler struct {
	eventRepo postgres.EventRepository
}

func (h feedHandler) Handle(ctx context.Context, event sqlc.Event) error {
	persisted, err := h.eventRepo.Add(ctx, event)
	if err != nil {
		return err
	}

	scheduleOn := persisted.CreatedAt.Add(
		time.Duration(viper.GetInt("GCLOUD_FEED_BUFFER_SECS")) * time.Second,
	)

	return task.CreateTaskForFeed(ctx, scheduleOn, task.FeedMessage{ID: persisted.ID})
}
