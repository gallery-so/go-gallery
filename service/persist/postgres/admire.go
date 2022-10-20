package postgres

import (
	"context"
	"database/sql"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

// AdmireRepository represents an admire repository in the postgres database
type AdmireRepository struct {
	db         *sql.DB
	queries    *db.Queries
	createStmt *sql.Stmt
	deleteStmt *sql.Stmt
}

// NewAdmireRepository creates a new postgres repository for interacting with admires
func NewAdmireRepository(db *sql.DB, queries *db.Queries) *AdmireRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx, `INSERT INTO admires (ID, FEED_EVENT_ID, ACTOR_ID) VALUES ($1, $2, $3) RETURNING ID;`)
	checkNoErr(err)

	deleteStmt, err := db.PrepareContext(ctx, `UPDATE admires SET DELETED = TRUE WHERE ID = $1;`)
	checkNoErr(err)

	return &AdmireRepository{
		db:         db,
		queries:    queries,
		createStmt: createStmt,
		deleteStmt: deleteStmt,
	}
}

func (a *AdmireRepository) CreateAdmire(ctx context.Context, feedEventID persist.DBID, actorID persist.DBID) (persist.DBID, error) {
	var resultID persist.DBID
	err := a.createStmt.QueryRowContext(ctx, persist.GenerateID(), feedEventID, actorID).Scan(&resultID)
	if err != nil {
		return "", err
	}
	return resultID, nil
}

func (a *AdmireRepository) RemoveAdmire(ctx context.Context, admireID persist.DBID) error {
	_, err := a.deleteStmt.ExecContext(ctx, admireID)
	return err
}
