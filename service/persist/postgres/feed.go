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

// LastPublishedCollectionFeedEvent returns the most recent collection event for the given owner, action, and collection that occurred before time `since`.
func (r *FeedRepository) LastPublishedCollectionFeedEvent(ctx context.Context, ownerID persist.DBID, collectionID persist.DBID, since time.Time, actions []persist.Action) (*db.FeedEvent, error) {
	evt, err := r.Queries.GetLastFeedEventForCollection(ctx, db.GetLastFeedEventForCollectionParams{
		OwnerID:      ownerID,
		Actions:      actionsToString(actions),
		CollectionID: string(collectionID),
		EventTime:    since,
	})

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	return &evt, err
}
