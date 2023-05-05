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
		GroupID:   event.GroupID,
	})

	return &evt, err
}

func (r *FeedRepository) LastPublishedUserFeedEvent(ctx context.Context, ownerID persist.DBID, before time.Time, actions []persist.Action) (*db.FeedEvent, error) {
	evt, err := r.Queries.GetLastFeedEventForUser(ctx, db.GetLastFeedEventForUserParams{
		OwnerID:   ownerID,
		Actions:   actions,
		EventTime: before,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &evt, err
}

func (r *FeedRepository) LastPublishedTokenFeedEvent(ctx context.Context, ownerID, tokenID persist.DBID, before time.Time, actions []persist.Action) (*db.FeedEvent, error) {
	evt, err := r.Queries.GetLastFeedEventForToken(ctx, db.GetLastFeedEventForTokenParams{
		OwnerID:   ownerID,
		TokenID:   tokenID.String(),
		Actions:   actions,
		EventTime: before,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &evt, err
}

// LastPublishedCollectionFeedEvent returns the most recent collection event for the given owner, action, and collection that occurred before time `since`.
func (r *FeedRepository) LastPublishedCollectionFeedEvent(ctx context.Context, ownerID persist.DBID, collectionID persist.DBID, before time.Time, actions []persist.Action) (*db.FeedEvent, error) {
	evt, err := r.Queries.GetLastFeedEventForCollection(ctx, db.GetLastFeedEventForCollectionParams{
		OwnerID:      ownerID,
		Actions:      actions,
		CollectionID: collectionID,
		EventTime:    before,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &evt, err
}
