package event

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/event/cloudtask"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

const EventHandlerContextKey = "event.eventHandlers"

type EventHandlers struct {
	Collection *CollectionDispatcher
	User       *UserDispatcher
	Nft        *NftDispatcher
}

func AddTo(ctx *gin.Context, repos *persist.Repositories) {
	collectionDispatcher := CollectionDispatcher{Handlers: map[persist.EventCode][]CollectionEventHandler{}}
	collectionTask := cloudtask.CollectionFeedTask{CollectionEventRepo: repos.CollectionEventRepository}
	collectionDispatcher.Handle(persist.CollectionCreatedEvent, &collectionTask)
	collectionDispatcher.Handle(persist.CollectionCollectorsNoteAdded, &collectionTask)
	collectionDispatcher.Handle(persist.CollectionTokensAdded, &collectionTask)

	userDispatcher := UserDispatcher{Handlers: map[persist.EventCode][]UserEventHandler{}}
	userTask := cloudtask.UserFeedTask{UserEventRepo: repos.UserEventRepository}
	userDispatcher.Handle(persist.UserCreatedEvent, &userTask)

	nftDispatcher := NftDispatcher{Handlers: map[persist.EventCode][]NftEventHandler{}}
	nftTask := cloudtask.NftFeedEvent{NftEventRepo: repos.NftEventRepository}
	nftDispatcher.Handle(persist.NftCollectorsNoteAddedEvent, &nftTask)

	eventHandlers := &EventHandlers{
		Collection: &collectionDispatcher,
		User:       &userDispatcher,
		Nft:        &nftDispatcher,
	}

	ctx.Set(EventHandlerContextKey, eventHandlers)
}

func For(ctx context.Context) *EventHandlers {
	gc := util.GinContextFromContext(ctx)
	return gc.Value(EventHandlerContextKey).(*EventHandlers)
}
