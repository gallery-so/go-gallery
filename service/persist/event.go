package persist

import (
	"context"
)

type EventType string

// Event represents a typically user-initiated event in the database.
type Event struct {
	ID           DBID            `json:"id"`
	UserID       DBID            `json:"user_id"`
	Version      NullInt32       `json:"version"`
	Type         EventType       `json:"event_type"`
	CreationTime CreationTime    `json:"created_at"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`
	Message      string          `json:"message"`
}

// EventRepository represents the interface for interacting with events.
type EventRepository interface {
	AddEvent(context.Context, Event) error
	GetEvent(context.Context, DBID) (Event, error)
	GetEventsSince(context.Context, EventType, DBID) ([]Event, error)
}
