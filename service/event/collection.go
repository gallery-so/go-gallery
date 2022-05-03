package event

import (
	"context"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/sentry"
)

type CollectionDispatcher struct {
	Handlers map[persist.EventCode][]CollectionEventHandler
}

func (c CollectionDispatcher) Handle(eventCode persist.EventCode, handler CollectionEventHandler) {
	c.Handlers[eventCode] = append(c.Handlers[eventCode], handler)
}

func (c CollectionDispatcher) Dispatch(ctx context.Context, event persist.CollectionEventRecord) {
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

type CollectionEventHandler interface {
	Handle(context.Context, persist.CollectionEventRecord)
}
