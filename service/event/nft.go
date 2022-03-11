package event

import "github.com/mikeydub/go-gallery/service/persist"

type NftDispatcher struct {
	Handlers map[persist.EventCode][]NftEventHandler
}

func (c NftDispatcher) Handle(eventCode persist.EventCode, handler NftEventHandler) {
	c.Handlers[eventCode] = append(c.Handlers[eventCode], handler)
}

func (c NftDispatcher) Dispatch(event persist.NftEventRecord) {
	go func() {
		if handlers, ok := c.Handlers[event.Code]; ok {
			for _, handler := range handlers {
				handler.Handle(event)
			}
		}
	}()
}

type NftEventHandler interface {
	Handle(persist.NftEventRecord)
}
