package postgres

import (
	"context"
	"database/sql"
	"time"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"

	"github.com/mikeydub/go-gallery/service/persist"
)

// CommentRepository represents an comment repository in the postgres database
type CommentRepository struct {
	db         *sql.DB
	queries    *db.Queries
	createStmt *sql.Stmt
	deleteStmt *sql.Stmt
}

// NewCommentRepository creates a new postgres repository for interacting with comments
func NewCommentRepository(db *sql.DB, queries *db.Queries) *CommentRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx, `INSERT INTO comments (ID, FEED_EVENT_ID, POST_ID, ACTOR_ID, REPLY_TO, COMMENT, MENTIONS) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING ID;`)
	checkNoErr(err)

	deleteStmt, err := db.PrepareContext(ctx, `UPDATE comments SET REMOVED = TRUE, COMMENT = 'comment removed' WHERE ID = $1;`)
	checkNoErr(err)

	return &CommentRepository{
		db:         db,
		queries:    queries,
		createStmt: createStmt,
		deleteStmt: deleteStmt,
	}
}

func (a *CommentRepository) CreateComment(ctx context.Context, feedEventID, postID, actorID persist.DBID, replyToID *persist.DBID, comment string, mentions map[persist.DBID]persist.Mention) (persist.DBID, error) {

	var feedEventString sql.NullString
	if feedEventID != "" {
		feedEventString = sql.NullString{
			String: feedEventID.String(),
			Valid:  true,
		}
	}

	var postString sql.NullString
	if postID != "" {
		postString = sql.NullString{
			String: postID.String(),
			Valid:  true,
		}
	}

	var resultID persist.DBID
	err := a.createStmt.QueryRowContext(ctx, persist.GenerateID(), feedEventString, postString, actorID, replyToID, comment, mentions).Scan(&resultID)
	if err != nil {
		return "", err
	}
	return resultID, nil
}

func (a *CommentRepository) RemoveComment(ctx context.Context, commentID persist.DBID) error {
	_, err := a.deleteStmt.ExecContext(ctx, commentID)
	return err
}
