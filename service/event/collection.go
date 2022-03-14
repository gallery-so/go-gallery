package event

import "github.com/mikeydub/go-gallery/service/persist"

type CollectionDispatcher struct {
	Handlers map[persist.EventCode][]CollectionEventHandler
}

func (c CollectionDispatcher) Handle(eventCode persist.EventCode, handler CollectionEventHandler) {
	c.Handlers[eventCode] = append(c.Handlers[eventCode], handler)
}

func (c CollectionDispatcher) Dispatch(event persist.CollectionEventRecord) {
	go func() {
		if handlers, ok := c.Handlers[event.Code]; ok {
			for _, handler := range handlers {
				handler.Handle(event)
			}
		}
	}()
}

type CollectionEventHandler interface {
	Handle(persist.CollectionEventRecord)
}
