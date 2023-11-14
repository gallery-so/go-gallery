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
	db                *sql.DB
	queries           *db.Queries
	createStmt        *sql.Stmt
	createMentionStmt *sql.Stmt
	deleteStmt        *sql.Stmt
	ancestorsStmt     *sql.Stmt
}

// NewCommentRepository creates a new postgres repository for interacting with comments
func NewCommentRepository(db *sql.DB, queries *db.Queries) *CommentRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx, `INSERT INTO comments (ID, FEED_EVENT_ID, POST_ID, ACTOR_ID, REPLY_TO, REPLY_ANCESTORS, COMMENT) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING ID;`)
	checkNoErr(err)

	createMentionStmt, err := db.PrepareContext(ctx, `INSERT INTO mentions (ID, COMMENT_ID, USER_ID, CONTRACT_ID, START, LENGTH) VALUES ($1, $2, $3, $4, $5, $6) RETURNING ID;`)
	checkNoErr(err)

	deleteStmt, err := db.PrepareContext(ctx, `UPDATE comments SET REMOVED = TRUE, COMMENT = 'comment removed' WHERE ID = $1;`)
	checkNoErr(err)

	ancestorsStmt, err := db.PrepareContext(ctx, `WITH RECURSIVE comment_thread AS (
    SELECT *
    FROM comments
    WHERE comments.id = $1
    UNION ALL
    SELECT c.* 
    FROM comments c
    INNER JOIN comment_thread ct ON c.id = ct.reply_to
)
SELECT id FROM comment_thread;`)
	checkNoErr(err)

	return &CommentRepository{
		db:                db,
		queries:           queries,
		createStmt:        createStmt,
		createMentionStmt: createMentionStmt,
		deleteStmt:        deleteStmt,
		ancestorsStmt:     ancestorsStmt,
	}
}

func (a *CommentRepository) CreateComment(ctx context.Context, feedEventID, postID, actorID persist.DBID, replyToID *persist.DBID, comment string, mentions []db.Mention) (persist.DBID, []db.Mention, error) {

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

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback()

	var ancestors []persist.DBID
	if replyToID != nil {
		rows, err := tx.StmtContext(ctx, a.ancestorsStmt).QueryContext(ctx, replyToID)
		if err != nil {
			return "", nil, err
		}
		for rows.Next() {
			var ancestor persist.DBID
			err = rows.Scan(&ancestor)
			if err != nil {
				return "", nil, err
			}

			ancestors = append(ancestors, ancestor)
		}
		if err = rows.Err(); err != nil {
			return "", nil, err
		}
	}

	var resultID persist.DBID
	err = tx.StmtContext(ctx, a.createStmt).QueryRowContext(ctx, persist.GenerateID(), feedEventString, postString, actorID, replyToID, ancestors, comment).Scan(&resultID)
	if err != nil {
		return "", nil, err
	}

	ms := make([]db.Mention, len(mentions))
	cm := tx.StmtContext(ctx, a.createMentionStmt)
	for i, m := range mentions {
		var userID, contractID sql.NullString
		if m.UserID != "" {
			userID = sql.NullString{
				String: m.UserID.String(),
				Valid:  true,
			}
		} else if m.ContractID != "" {
			contractID = sql.NullString{
				String: m.ContractID.String(),
				Valid:  true,
			}
		}
		var mid persist.DBID
		err = cm.QueryRowContext(ctx, persist.GenerateID(), resultID, userID, contractID, m.Start, m.Length).Scan(&mid)
		if err != nil {
			return "", nil, err
		}
		m.ID = mid
		ms[i] = m
	}

	err = tx.Commit()
	if err != nil {
		return "", nil, err
	}

	return resultID, ms, nil
}

func (a *CommentRepository) RemoveComment(ctx context.Context, commentID persist.DBID) error {
	_, err := a.deleteStmt.ExecContext(ctx, commentID)
	return err
}
