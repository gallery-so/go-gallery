// TODO: Remove when the feedbot uses the feed API instead of creating its own posts.
// Everything below can be removed.
package event

import (
	"context"

	"github.com/mikeydub/go-gallery/service/persist"
)

type NftDispatcher struct {
	Handlers map[persist.EventCode][]NftEventHandler
}

func (c NftDispatcher) Handle(eventCode persist.EventCode, handler NftEventHandler) {
	c.Handlers[eventCode] = append(c.Handlers[eventCode], handler)
}

func (c NftDispatcher) Dispatch(ctx context.Context, event persist.NftEventRecord) {
	if handlers, ok := c.Handlers[event.Code]; ok {
		for _, handler := range handlers {
			handler.Handle(ctx, event)
		}
	}
}

type NftEventHandler interface {
	Handle(context.Context, persist.NftEventRecord)
}
