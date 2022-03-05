package postgres

import (
	"context"
	"database/sql"

	"github.com/mikeydub/go-gallery/service/persist"
)

type EventRepository struct {
}

func NewEventRepository(db *sql.DB) *EventRepository {
	return &EventRepository{}
}

func (e *EventRepository) AddEvent(ctx context.Context, event persist.Event) error {
	panic("not implemented")
}

func (e *EventRepository) GetEvent(ctx context.Context, eventID persist.DBID) (persist.Event, error) {
	panic("not implemented")
}

func (e *EventRepository) GetEventsSince(ctx context.Context, eventType persist.EventType, eventID persist.DBID) ([]persist.Event, error) {
	panic("not implemented")
}
