package postgres

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/spf13/viper"
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
		UserID:         event.SubjectID,
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
		TokenID:        event.SubjectID,
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
		CollectionID:   event.SubjectID,
		Data:           event.Data,
	})
	return &event, err
}

// WindowActiveForActorAction checks if there are more recent events with the same actor and action as the provided event.
func (r *EventRepository) WindowActiveForActorAction(ctx context.Context, event sqlc.Event) (bool, error) {
	return r.Queries.IsWindowActiveForActorAction(ctx, sqlc.IsWindowActiveForActorActionParams{
		ActorID:     event.ActorID,
		Action:      event.Action,
		WindowStart: event.CreatedAt,
		WindowEnd:   event.CreatedAt.Add(time.Duration(viper.GetInt("FEED_WINDOW_SIZE")) * time.Second),
	})
}

// WindowActiveForActorActionSubject checks if there are more recent events with the same actor and action applied to a specific subject such as
// for a particular collection or token.
func (r *EventRepository) WindowActiveForActorActionSubject(ctx context.Context, event sqlc.Event) (bool, error) {
	return r.Queries.IsWindowActiveForActorActionSubject(ctx, sqlc.IsWindowActiveForActorActionSubjectParams{
		ActorID:     event.ActorID,
		Action:      event.Action,
		SubjectID:   event.SubjectID,
		WindowStart: event.CreatedAt,
		WindowEnd:   event.CreatedAt.Add(time.Duration(viper.GetInt("FEED_WINDOW_SIZE")) * time.Second),
	})
}

// WindowActiveForActionSubject checks if there are more recent events with the same action and subject as the provided event.
func (r *EventRepository) WindowActiveForActionSubject(ctx context.Context, event sqlc.Event) (bool, error) {
	return r.Queries.IsWindowActiveForActionSubject(ctx, sqlc.IsWindowActiveForActionSubjectParams{
		Action:      event.Action,
		SubjectID:   event.SubjectID,
		WindowStart: event.CreatedAt,
		WindowEnd:   event.CreatedAt.Add(time.Duration(viper.GetInt("FEED_WINDOW_SIZE")) * time.Second),
	})
}

// EventsInWindowForActorAction returns events with the same actor and action that belong to the same window of activity as the given eventID.
func (r *EventRepository) EventsInWindowForActorAction(ctx context.Context, eventID persist.DBID, windowSeconds int) ([]sqlc.Event, error) {
	return r.Queries.GetEventsInWindowForActorAction(ctx, sqlc.GetEventsInWindowForActorActionParams{
		ID:   eventID,
		Secs: float64(windowSeconds),
	})
}

// EventsInWindowForActionSubject returns events with the same subject and action that belong to the same window of activity as the given eventID
// regardless of the actor.
func (r *EventRepository) EventsInWindowForActionSubject(ctx context.Context, eventID persist.DBID, windowSeconds int) ([]sqlc.Event, error) {
	return r.Queries.GetEventsInWindowForActionSubject(ctx, sqlc.GetEventsInWindowForActionSubjectParams{
		ID:   eventID,
		Secs: float64(windowSeconds),
	})
}
