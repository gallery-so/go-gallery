// TODO: Remove when the feedbot uses the feed API instead of creating its own posts.
// Everything below can be removed.
package event

import (
	"context"

	"github.com/mikeydub/go-gallery/service/persist"
)

type CollectionDispatcher struct {
	Handlers map[persist.EventCode][]CollectionEventHandler
}

func (c CollectionDispatcher) Handle(eventCode persist.EventCode, handler CollectionEventHandler) {
	c.Handlers[eventCode] = append(c.Handlers[eventCode], handler)
}

func (c CollectionDispatcher) Dispatch(ctx context.Context, event persist.CollectionEventRecord) {
	if handlers, ok := c.Handlers[event.Code]; ok {
		for _, handler := range handlers {
			handler.Handle(ctx, event)
		}
	}
}

type CollectionEventHandler interface {
	Handle(context.Context, persist.CollectionEventRecord)
}
