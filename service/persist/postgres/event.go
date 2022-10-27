package postgres

import (
	"context"
	"time"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/spf13/viper"
)

type EventRepository struct {
	Queries *db.Queries
}

func (r *EventRepository) Get(ctx context.Context, eventID persist.DBID) (db.Event, error) {
	return r.Queries.GetEvent(ctx, eventID)
}

func (r *EventRepository) Add(ctx context.Context, event db.Event) (*db.Event, error) {
	switch event.ResourceTypeID {
	case persist.ResourceTypeUser:
		return r.AddUserEvent(ctx, event)
	case persist.ResourceTypeToken:
		return r.AddTokenEvent(ctx, event)
	case persist.ResourceTypeCollection:
		return r.AddCollectionEvent(ctx, event)
	case persist.ResourceTypeAdmire:
		return r.AddAdmireEvent(ctx, event)
	case persist.ResourceTypeComment:
		return r.AddCommentEvent(ctx, event)
	case persist.ResourceTypeGallery:
		return r.AddGalleryEvent(ctx, event)
	default:
		return nil, persist.ErrUnknownResourceType{ResourceType: event.ResourceTypeID}
	}
}

func (r *EventRepository) AddUserEvent(ctx context.Context, event db.Event) (*db.Event, error) {
	event, err := r.Queries.CreateUserEvent(ctx, db.CreateUserEventParams{
		ID:             persist.GenerateID(),
		ActorID:        event.ActorID,
		Action:         event.Action,
		ResourceTypeID: event.ResourceTypeID,
		UserID:         event.SubjectID,
		Data:           event.Data,
	})
	return &event, err
}

func (r *EventRepository) AddTokenEvent(ctx context.Context, event db.Event) (*db.Event, error) {
	event, err := r.Queries.CreateTokenEvent(ctx, db.CreateTokenEventParams{
		ID:             persist.GenerateID(),
		ActorID:        event.ActorID,
		Action:         event.Action,
		ResourceTypeID: event.ResourceTypeID,
		TokenID:        event.SubjectID,
		Data:           event.Data,
	})
	return &event, err
}

func (r *EventRepository) AddCollectionEvent(ctx context.Context, event db.Event) (*db.Event, error) {
	event, err := r.Queries.CreateCollectionEvent(ctx, db.CreateCollectionEventParams{
		ID:             persist.GenerateID(),
		ActorID:        event.ActorID,
		Action:         event.Action,
		ResourceTypeID: event.ResourceTypeID,
		CollectionID:   event.SubjectID,
		Data:           event.Data,
		Caption:        event.Caption,
	})
	return &event, err
}

func (r *EventRepository) AddAdmireEvent(ctx context.Context, event db.Event) (*db.Event, error) {
	event, err := r.Queries.CreateAdmireEvent(ctx, db.CreateAdmireEventParams{
		ID:             persist.GenerateID(),
		ActorID:        event.ActorID,
		Action:         event.Action,
		ResourceTypeID: event.ResourceTypeID,
		AdmireID:       event.AdmireID,
		FeedEventID:    event.FeedEventID,
		Data:           event.Data,
	})
	return &event, err
}

func (r *EventRepository) AddCommentEvent(ctx context.Context, event db.Event) (*db.Event, error) {
	event, err := r.Queries.CreateCommentEvent(ctx, db.CreateCommentEventParams{
		ID:             persist.GenerateID(),
		ActorID:        event.ActorID,
		Action:         event.Action,
		ResourceTypeID: event.ResourceTypeID,
		CommentID:      event.CommentID,
		FeedEventID:    event.FeedEventID,
		Data:           event.Data,
	})
	return &event, err
}

func (r *EventRepository) AddGalleryEvent(ctx context.Context, event db.Event) (*db.Event, error) {
	event, err := r.Queries.CreateGalleryEvent(ctx, db.CreateGalleryEventParams{
		ID:             persist.GenerateID(),
		ActorID:        event.ActorID,
		Action:         event.Action,
		ResourceTypeID: event.ResourceTypeID,
		GalleryID:      event.SubjectID,
		Data:           event.Data,
	})
	return &event, err
}

func (r *EventRepository) IsActorActionActive(ctx context.Context, event db.Event, actions []persist.Action) (bool, error) {
	windowStart, windowEnd := windowBounds(event)
	return r.Queries.IsActorActionActive(ctx, db.IsActorActionActiveParams{
		ActorID:     event.ActorID,
		Actions:     actionsToString(actions),
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	})
}

func (r *EventRepository) IsActorSubjectActive(ctx context.Context, event db.Event) (bool, error) {
	windowStart, windowEnd := windowBounds(event)
	return r.Queries.IsActorSubjectActive(ctx, db.IsActorSubjectActiveParams{
		ActorID:     event.ActorID,
		SubjectID:   event.SubjectID,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	})
}

func (r *EventRepository) IsActorSubjectActionActive(ctx context.Context, event db.Event, actions []persist.Action) (bool, error) {
	windowStart, windowEnd := windowBounds(event)
	return r.Queries.IsActorSubjectActionActive(ctx, db.IsActorSubjectActionActiveParams{
		ActorID:     event.ActorID,
		SubjectID:   event.SubjectID,
		Actions:     actionsToString(actions),
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	})
}

// EventsInWindow returns events belonging to the same window of activity as the given eventID.
func (r *EventRepository) EventsInWindow(ctx context.Context, eventID persist.DBID, windowSeconds int, actions ...persist.Action) ([]db.Event, error) {
	return r.Queries.GetEventsInWindow(ctx, db.GetEventsInWindowParams{
		ID:      eventID,
		Secs:    float64(windowSeconds),
		Actions: actionsToString(actions),
	})
}

func windowBounds(event db.Event) (start, end time.Time) {
	return event.CreatedAt, event.CreatedAt.Add(time.Duration(viper.GetInt("FEED_WINDOW_SIZE")) * time.Second)
}

func actionsToString(actions []persist.Action) []string {
	actionsAsStr := make([]string, len(actions))
	for i, action := range actions {
		actionsAsStr[i] = string(action)
	}
	return actionsAsStr
}
