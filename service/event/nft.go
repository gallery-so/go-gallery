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

	go func(hubCtx context.Context) {
		if handlers, ok := c.Handlers[event.Code]; ok {
			for _, handler := range handlers {
				handler.Handle(hubCtx, event)
			}
		}
	}(sentryutil.NewSentryHubContext(ctx, currentHub))
}

type NftEventHandler interface {
	Handle(context.Context, persist.NftEventRecord)
}
