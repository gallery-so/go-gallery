package event

import (
	"fmt"

	"github.com/mikeydub/go-gallery/service/persist"
)

// Represents a message to be added to a task queue.
type EventMessage struct {
	ID        persist.DBID
	EventCode persist.EventCode
}

type EventHandler struct {
	// Channel of persisted events
	events <-chan persist.DBID
}

func NewEventHandler(ch chan persist.DBID) EventHandler {
	return EventHandler{events: ch}
}

func (e *EventHandler) Handle() {
	for {
		e, ok := <-e.events
		if ok {
			fmt.Println(e)
			// Send to task queue.
		} else {
			return
		}
	}
}
