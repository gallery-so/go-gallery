package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/service/persist"
)

type FeedRepository struct {
	Queries *sqlc.Queries
}

func (r *FeedRepository) Add(ctx context.Context, event sqlc.FeedEvent) (*sqlc.FeedEvent, error) {
	evt, err := r.Queries.CreateFeedEvent(ctx, sqlc.CreateFeedEventParams{
		ID:        persist.GenerateID(),
		OwnerID:   event.OwnerID,
		Action:    event.Action,
		Data:      event.Data,
		EventTime: event.EventTime,
		EventIds:  event.EventIds,
	})

	return &evt, err
}

// LastEventFrom returns the most recent event which occurred before `event`.
func (r *FeedRepository) LastEventFrom(ctx context.Context, event sqlc.Event) (*sqlc.FeedEvent, error) {
	evt, err := r.Queries.GetLastFeedEvent(ctx, sqlc.GetLastFeedEventParams{
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
func (r *FeedRepository) LastTokenEventFromEvent(ctx context.Context, event sqlc.Event) (*sqlc.FeedEvent, error) {
	evt, err := r.Queries.GetLastFeedEventForToken(ctx, sqlc.GetLastFeedEventForTokenParams{
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
func (r *FeedRepository) LastCollectionEventFromEvent(ctx context.Context, event sqlc.Event) (*sqlc.FeedEvent, error) {
	return r.LastCollectionEvent(ctx, event.ActorID, event.Action, event.SubjectID, event.CreatedAt)
}

// LastCollectionEvent returns the most recent collection event for the given owner, action, and collection that occurred before time `since`.
func (r *FeedRepository) LastCollectionEvent(ctx context.Context, ownerID persist.DBID, action persist.Action, collectionID persist.DBID, since time.Time) (*sqlc.FeedEvent, error) {
	evt, err := r.Queries.GetLastFeedEventForCollection(ctx, sqlc.GetLastFeedEventForCollectionParams{
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
