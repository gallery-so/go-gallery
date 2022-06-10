package postgres

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/service/persist"
)

type EventRepository struct {
	Queries *sqlc.Queries
}

func (r *EventRepository) Get(ctx context.Context, eventID persist.DBID) (sqlc.Event, error) {
	return r.Queries.GetEvent(ctx, eventID)
}

func (r *EventRepository) Add(ctx context.Context, event sqlc.Event) (sqlc.Event, error) {
	return r.Queries.CreateEvent(ctx, sqlc.CreateEventParams{
		ID:        persist.GenerateID(),
		ActorID:   event.ActorID,
		Action:    event.Action,
		SubjectID: event.SubjectID,
		Data:      event.Data,
	})
}

// WindowActive checks if there are more recent events with an action that matches the provided event.
func (r *EventRepository) WindowActive(ctx context.Context, event sqlc.Event, since time.Time) (bool, error) {
	return r.Queries.IsWindowActive(ctx, sqlc.IsWindowActiveParams{
		ActorID:   event.ActorID,
		Action:    event.Action,
		Timestart: event.CreatedAt,
		Timeend:   since,
	})
}

// WindowActiveForSubject checks if there are more recent events with an action on a specific resource such as
// as a collection or a token.
func (r *EventRepository) WindowActiveForSubject(ctx context.Context, event sqlc.Event, since time.Time) (bool, error) {
	return r.Queries.IsWindowActiveWithSubject(ctx, sqlc.IsWindowActiveWithSubjectParams{
		ActorID:   event.ActorID,
		Action:    event.Action,
		SubjectID: event.SubjectID,
		Timestart: event.CreatedAt,
		Timeend:   since,
	})
}

// EventsInWindow returns events belonging to the same window of activity.
func (r *EventRepository) EventsInWindow(ctx context.Context, actorID persist.DBID, action persist.Action, windowStart time.Time, windowEnd time.Time) ([]sqlc.Event, error) {
	return r.Queries.GetEventsInWindow(ctx, sqlc.GetEventsInWindowParams{
		ActorID:   actorID,
		Action:    action,
		Timestart: windowStart,
		Timeend:   windowEnd,
	})
}
