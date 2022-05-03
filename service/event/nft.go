package event

import (
	"context"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/sentry"
)

type NftDispatcher struct {
	Handlers map[persist.EventCode][]NftEventHandler
}

func (c NftDispatcher) Handle(eventCode persist.EventCode, handler NftEventHandler) {
	c.Handlers[eventCode] = append(c.Handlers[eventCode], handler)
}

func (c NftDispatcher) Dispatch(ctx context.Context, event persist.NftEventRecord) {
	currentHub := sentryutil.SentryHubFromContext(ctx)

	go func() {
		ctx := sentryutil.NewSentryHubContext(ctx, currentHub)

		if handlers, ok := c.Handlers[event.Code]; ok {
			for _, handler := range handlers {
				handler.Handle(ctx, event)
			}
		}
	}()
}

type NftEventHandler interface {
	Handle(context.Context, persist.NftEventRecord)
}
