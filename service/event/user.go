package event

import (
	"context"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/sentry"
)

type UserDispatcher struct {
	Handlers map[persist.EventCode][]UserEventHandler
}

func (c UserDispatcher) Handle(eventCode persist.EventCode, handler UserEventHandler) {
	c.Handlers[eventCode] = append(c.Handlers[eventCode], handler)
}

func (c UserDispatcher) Dispatch(ctx context.Context, event persist.UserEventRecord) {
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

type UserEventHandler interface {
	Handle(context.Context, persist.UserEventRecord)
}
