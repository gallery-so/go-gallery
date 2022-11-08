package postgres

import (
	"context"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

// AdmireRepository represents an admire repository in the postgres database
type AdmireRepository struct {
	queries *db.Queries
}

// NewAdmireRepository creates a new postgres repository for interacting with admires
func NewAdmireRepository(queries *db.Queries) *AdmireRepository {
	return &AdmireRepository{
		queries: queries,
	}
}

func (a *AdmireRepository) CreateAdmire(ctx context.Context, feedEventID persist.DBID, actorID persist.DBID) (persist.DBID, error) {
	admireID, err := a.queries.CreateAdmire(ctx, db.CreateAdmireParams{
		ID:          persist.GenerateID(),
		FeedEventID: feedEventID,
		ActorID:     actorID,
	})

	if err != nil {
		return "", err
	}

	return admireID, nil
}

func (a *AdmireRepository) RemoveAdmire(ctx context.Context, admireID persist.DBID) error {
	return a.queries.DeleteAdmireByID(ctx, admireID)
}
