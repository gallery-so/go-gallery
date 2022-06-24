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

func (r *EventRepository) Add(ctx context.Context, event sqlc.Event) (*sqlc.Event, error) {
	switch event.ResourceTypeID {
	case persist.ResourceTypeUser:
		return r.AddUserEvent(ctx, event)
	case persist.ResourceTypeToken:
		return r.AddTokenEvent(ctx, event)
	case persist.ResourceTypeCollection:
		return r.AddCollectionEvent(ctx, event)
	default:
		return nil, persist.ErrUnknownResourceType{ResourceType: event.ResourceTypeID}
	}
}

func (r *EventRepository) AddUserEvent(ctx context.Context, event sqlc.Event) (*sqlc.Event, error) {
	event, err := r.Queries.CreateUserEvent(ctx, sqlc.CreateUserEventParams{
		ID:             persist.GenerateID(),
		ActorID:        event.ActorID,
		Action:         event.Action,
		ResourceTypeID: event.ResourceTypeID,
		UserID:         event.UserID,
		SubjectID:      event.SubjectID,
		Data:           event.Data,
	})
	return &event, err
}

func (r *EventRepository) AddTokenEvent(ctx context.Context, event sqlc.Event) (*sqlc.Event, error) {
	event, err := r.Queries.CreateTokenEvent(ctx, sqlc.CreateTokenEventParams{
		ID:             persist.GenerateID(),
		ActorID:        event.ActorID,
		Action:         event.Action,
		ResourceTypeID: event.ResourceTypeID,
		TokenID:        event.TokenID,
		SubjectID:      event.SubjectID,
		Data:           event.Data,
	})
	return &event, err
}

func (r *EventRepository) AddCollectionEvent(ctx context.Context, event sqlc.Event) (*sqlc.Event, error) {
	event, err := r.Queries.CreateCollectionEvent(ctx, sqlc.CreateCollectionEventParams{
		ID:             persist.GenerateID(),
		ActorID:        event.ActorID,
		Action:         event.Action,
		ResourceTypeID: event.ResourceTypeID,
		CollectionID:   event.CollectionID,
		SubjectID:      event.SubjectID,
		Data:           event.Data,
	})
	return &event, err
}

// WindowActive checks if there are more recent events with an action that matches the provided event.
func (r *EventRepository) WindowActive(ctx context.Context, event sqlc.Event, since time.Time) (bool, error) {
	return r.Queries.IsWindowActive(ctx, sqlc.IsWindowActiveParams{
		ActorID:   event.ActorID,
		Action:    event.Action,
		TimeStart: event.CreatedAt,
		TimeEnd:   since,
	})
}

// WindowActiveForSubject checks if there are more recent events with an action on a specific resource such as
// as a collection or a token.
func (r *EventRepository) WindowActiveForSubject(ctx context.Context, event sqlc.Event, since time.Time) (bool, error) {
	return r.Queries.IsWindowActiveWithSubject(ctx, sqlc.IsWindowActiveWithSubjectParams{
		ActorID:   event.ActorID,
		Action:    event.Action,
		SubjectID: event.SubjectID,
		TimeStart: event.CreatedAt,
		TimeEnd:   since,
	})
}

// EventsInWindow returns events belonging to the same window of activity.
func (r *EventRepository) EventsInWindow(ctx context.Context, actorID persist.DBID, action persist.Action, windowStart time.Time, windowEnd time.Time) ([]sqlc.Event, error) {
	return r.Queries.GetEventsInWindow(ctx, sqlc.GetEventsInWindowParams{
		ActorID:   actorID,
		Action:    action,
		TimeStart: windowStart,
		TimeEnd:   windowEnd,
	})
}
