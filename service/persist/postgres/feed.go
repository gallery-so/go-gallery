package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v4"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

type FeedRepository struct {
	Queries *db.Queries
}

func (r *FeedRepository) Add(ctx context.Context, event db.FeedEvent) (*db.FeedEvent, error) {
	evt, err := r.Queries.CreateFeedEvent(ctx, db.CreateFeedEventParams{
		ID:        persist.GenerateID(),
		OwnerID:   event.OwnerID,
		Action:    event.Action,
		Data:      event.Data,
		EventTime: event.EventTime,
		EventIds:  event.EventIds,
		Caption:   event.Caption,
	})

	return &evt, err
}

func (r *FeedRepository) AddCaptionToEvent(ctx context.Context, userID, feedEventID persist.DBID, caption string) (bool, error) {
	affected, err := r.Queries.AddFeedCaption(ctx, db.AddFeedCaptionParams{
		OwnerID: userID,
		ID:      feedEventID,
		Caption: caption,
	})
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// LastEventFrom returns the most recent event which occurred before `event`.
func (r *FeedRepository) LastEventFrom(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	evt, err := r.Queries.GetLastFeedEvent(ctx, db.GetLastFeedEventParams{
		OwnerID:   event.ActorID,
		Action:    event.Action,
		EventTime: event.CreatedAt,
	})

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	return &evt, err
}

// LastTokenEventFromEvent returns the most recent token event which occured before `event`.
func (r *FeedRepository) LastTokenEventFromEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	evt, err := r.Queries.GetLastFeedEventForToken(ctx, db.GetLastFeedEventForTokenParams{
		OwnerID:   event.ActorID,
		Action:    event.Action,
		TokenID:   string(event.SubjectID),
		EventTime: event.CreatedAt,
	})

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	return &evt, err
}

// LastCollectionEventFromEvent returns the most recent collection event which occurred before `event`.
func (r *FeedRepository) LastCollectionEventFromEvent(ctx context.Context, event db.Event) (*db.FeedEvent, error) {
	return r.LastCollectionEvent(ctx, event.ActorID, event.Action, event.SubjectID, event.CreatedAt)
}

// LastCollectionEvent returns the most recent collection event for the given owner, action, and collection that occurred before time `since`.
func (r *FeedRepository) LastCollectionEvent(ctx context.Context, ownerID persist.DBID, action persist.Action, collectionID persist.DBID, since time.Time) (*db.FeedEvent, error) {
	evt, err := r.Queries.GetLastFeedEventForCollection(ctx, db.GetLastFeedEventForCollectionParams{
		OwnerID:      ownerID,
		Action:       action,
		CollectionID: string(collectionID),
		EventTime:    since,
	})

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	return &evt, err
}
