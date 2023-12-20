package postgres

import (
	"context"
	"time"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
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
	case persist.ResourceTypeContract:
		return r.AddContractEvent(ctx, event)
	case persist.ResourceTypePost:
		return r.AddPostEvent(ctx, event)
	case persist.ResourceTypeAllUsers:
		return r.AddDataOnlyEvent(ctx, event)
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
		Post:           util.ToNullString(event.PostID.String(), true),
		Comment:        util.ToNullString(event.CommentID.String(), true),
		FeedEvent:      util.ToNullString(event.FeedEventID.String(), true),
		Mention:        util.ToNullString(event.MentionID.String(), true),
		UserID:         event.SubjectID,
		Data:           event.Data,
		GroupID:        event.GroupID,
		Caption:        event.Caption,
	})
	return &event, err
}

func (r *EventRepository) AddTokenEvent(ctx context.Context, event db.Event) (*db.Event, error) {
	gid := util.StringToPointerIfNotEmpty(event.GalleryID.String())
	cid := util.StringToPointerIfNotEmpty(event.CollectionID.String())
	event, err := r.Queries.CreateTokenEvent(ctx, db.CreateTokenEventParams{
		ID:             persist.GenerateID(),
		ActorID:        event.ActorID,
		Action:         event.Action,
		ResourceTypeID: event.ResourceTypeID,
		TokenID:        event.SubjectID,
		Data:           event.Data,
		GroupID:        event.GroupID,
		Caption:        event.Caption,
		Gallery:        persist.StrPtrToNullStr(gid),
		Collection:     persist.StrPtrToNullStr(cid),
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
		GroupID:        event.GroupID,
		GalleryID:      event.GalleryID,
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
		FeedEvent:      util.ToNullString(event.FeedEventID.String(), true),
		Post:           util.ToNullString(event.PostID.String(), true),
		SubjectID:      event.SubjectID,
		Data:           event.Data,
		GroupID:        event.GroupID,
		Caption:        event.Caption,
		Token:          util.ToNullString(event.TokenID.String(), true),
		Comment:        util.ToNullString(event.CommentID.String(), true),
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
		FeedEvent:      util.ToNullString(event.FeedEventID.String(), true),
		Post:           util.ToNullString(event.PostID.String(), true),
		Data:           event.Data,
		GroupID:        event.GroupID,
		Caption:        event.Caption,
		SubjectID:      event.SubjectID,
		Mention:        util.ToNullString(event.MentionID.String(), true),
	})
	return &event, err
}

func (r *EventRepository) AddGalleryEvent(ctx context.Context, event db.Event) (*db.Event, error) {
	event, err := r.Queries.CreateGalleryEvent(ctx, db.CreateGalleryEventParams{
		ID:             persist.GenerateID(),
		ActorID:        event.ActorID,
		Action:         event.Action,
		ResourceTypeID: event.ResourceTypeID,
		GalleryID:      event.GalleryID,
		Data:           event.Data,
		ExternalID:     event.ExternalID,
		GroupID:        event.GroupID,
		Caption:        event.Caption,
	})
	return &event, err
}

func (r *EventRepository) AddContractEvent(ctx context.Context, event db.Event) (*db.Event, error) {
	event, err := r.Queries.CreateContractEvent(ctx, db.CreateContractEventParams{
		Post:           util.ToNullString(event.PostID.String(), true),
		Comment:        util.ToNullString(event.CommentID.String(), true),
		FeedEvent:      util.ToNullString(event.FeedEventID.String(), true),
		Mention:        util.ToNullString(event.MentionID.String(), true),
		ContractID:     event.ContractID,
		Data:           event.Data,
		GroupID:        event.GroupID,
		Caption:        event.Caption,
		Action:         event.Action,
		ActorID:        event.ActorID,
		ID:             persist.GenerateID(),
		ResourceTypeID: event.ResourceTypeID,
	})
	return &event, err
}

func (r *EventRepository) AddPostEvent(ctx context.Context, event db.Event) (*db.Event, error) {
	event, err := r.Queries.CreatePostEvent(ctx, db.CreatePostEventParams{
		ID:             persist.GenerateID(),
		ActorID:        event.ActorID,
		Action:         event.Action,
		ResourceTypeID: event.ResourceTypeID,
		UserID:         event.UserID,
		SubjectID:      event.SubjectID,
		PostID:         event.PostID,
	})
	return &event, err
}

func (r *EventRepository) AddDataOnlyEvent(ctx context.Context, event db.Event) (*db.Event, error) {
	event, err := r.Queries.CreateDataOnlyEvent(ctx, db.CreateDataOnlyEventParams{
		ID:             persist.GenerateID(),
		ActorID:        event.ActorID,
		Action:         event.Action,
		ResourceTypeID: event.ResourceTypeID,
		Data:           event.Data,
		GroupID:        event.GroupID,
		Caption:        event.Caption,
		SubjectID:      event.SubjectID,
	})
	return &event, err
}

func (r *EventRepository) IsActorActionActive(ctx context.Context, event db.Event, actions persist.ActionList, windowSize time.Duration) (bool, error) {
	return r.Queries.IsActorActionActive(ctx, db.IsActorActionActiveParams{
		ActorID:     event.ActorID,
		Actions:     actions,
		WindowStart: event.CreatedAt,
		WindowEnd:   event.CreatedAt.Add(windowSize),
	})
}

func (r *EventRepository) IsActorSubjectActive(ctx context.Context, event db.Event, windowSize time.Duration) (bool, error) {
	return r.Queries.IsActorSubjectActive(ctx, db.IsActorSubjectActiveParams{
		ActorID:     event.ActorID,
		SubjectID:   event.SubjectID,
		WindowStart: event.CreatedAt,
		WindowEnd:   event.CreatedAt.Add(windowSize),
	})
}

func (r *EventRepository) IsActorGalleryActive(ctx context.Context, event db.Event, windowSize time.Duration) (bool, error) {
	return r.Queries.IsActorGalleryActive(ctx, db.IsActorGalleryActiveParams{
		ActorID:     event.ActorID,
		GalleryID:   event.GalleryID,
		WindowStart: event.CreatedAt,
		WindowEnd:   event.CreatedAt.Add(windowSize),
	})
}

func (r *EventRepository) IsActorSubjectActionActive(ctx context.Context, event db.Event, actions persist.ActionList, windowSize time.Duration) (bool, error) {
	return r.Queries.IsActorSubjectActionActive(ctx, db.IsActorSubjectActionActiveParams{
		ActorID:     event.ActorID,
		SubjectID:   event.SubjectID,
		Actions:     actions,
		WindowStart: event.CreatedAt,
		WindowEnd:   event.CreatedAt.Add(windowSize),
	})
}

// EventsInWindow returns events belonging to the same window of activity as the given eventID.
func (r *EventRepository) EventsInWindow(ctx context.Context, eventID persist.DBID, windowSeconds int, actions persist.ActionList, includeSubject bool) ([]db.Event, error) {
	return r.Queries.GetEventsInWindow(ctx, db.GetEventsInWindowParams{
		ID:             eventID,
		Secs:           float64(windowSeconds),
		Actions:        actions,
		IncludeSubject: includeSubject,
	})
}

// EventsInWindow returns events belonging to the same window of activity as the given eventID.
func (r *EventRepository) EventsInWindowForGallery(ctx context.Context, eventID, galleryID persist.DBID, windowSeconds int, actions persist.ActionList, includeSubject bool) ([]db.Event, error) {
	return r.Queries.GetGalleryEventsInWindow(ctx, db.GetGalleryEventsInWindowParams{
		ID:             eventID,
		Secs:           float64(windowSeconds),
		Actions:        actions,
		IncludeSubject: includeSubject,
		GalleryID:      galleryID,
	})
}
