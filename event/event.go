package event

import (
	"fmt"

	"github.com/mikeydub/go-gallery/service/persist"
)

// Represents a message added to a task queue.
type EventMessage struct {
	ID        persist.DBID
	EventType persist.EventType
}

type EventHandler struct {
	// Channel of persisted events
	Events <-chan persist.DBID
}

func (e *EventHandler) Handle() {
	for {
		e, ok := <-e.Events
		if ok {
			fmt.Println(e)
			// Sent to task queue.
		} else {
			return
		}
	}
}
