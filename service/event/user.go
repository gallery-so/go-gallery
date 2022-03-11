package event

import "github.com/mikeydub/go-gallery/service/persist"

type UserDispatcher struct {
	Handlers map[persist.EventCode][]UserEventHandler
}

func (c UserDispatcher) Handle(eventCode persist.EventCode, handler UserEventHandler) {
	c.Handlers[eventCode] = append(c.Handlers[eventCode], handler)
}

func (c UserDispatcher) Dispatch(event persist.UserEventRecord) {
	go func() {
		if handlers, ok := c.Handlers[event.Code]; ok {
			for _, handler := range handlers {
				handler.Handle(event)
			}
		}
	}()
}

type UserEventHandler interface {
	Handle(persist.UserEventRecord)
}
