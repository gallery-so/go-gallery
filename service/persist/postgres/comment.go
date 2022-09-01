package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

// CommentRepository represents an comment repository in the postgres database
type CommentRepository struct {
	db         *sql.DB
	createStmt *sql.Stmt
	deleteStmt *sql.Stmt
}

// NewCommentRepository creates a new postgres repository for interacting with comments
func NewCommentRepository(db *sql.DB) *CommentRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx, `INSERT INTO comments (ID, FEED_EVENT_ID, ACTOR_ID, REPLY_TO, COMMENT) VALUES ($1, $2, $3, $4, $5) RETURNING ID;`)
	checkNoErr(err)

	deleteStmt, err := db.PrepareContext(ctx, `UPDATE comments SET DELETED = TRUE WHERE ID = $1;`)
	checkNoErr(err)

	return &CommentRepository{
		db:         db,
		createStmt: createStmt,
		deleteStmt: deleteStmt,
	}
}

func (a *CommentRepository) CreateComment(ctx context.Context, feedEventID, actorID persist.DBID, replyToID *persist.DBID, comment string) (persist.DBID, error) {
	var resultID persist.DBID
	err := a.createStmt.QueryRowContext(ctx, persist.GenerateID(), feedEventID, actorID, replyToID, comment).Scan(&resultID)
	if err != nil {
		return "", err
	}
	return resultID, nil
}

func (a *CommentRepository) RemoveComment(ctx context.Context, commentID persist.DBID) error {
	_, err := a.deleteStmt.ExecContext(ctx, commentID)
	return err
}
