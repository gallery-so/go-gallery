package postgres

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v4/pgxpool"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"

	"github.com/mikeydub/go-gallery/service/persist"
)

// CommentRepository represents an comment repository in the postgres database
type CommentRepository struct {
	pgx     *pgxpool.Pool
	queries *db.Queries
}

// NewCommentRepository creates a new postgres repository for interacting with comments
func NewCommentRepository(queries *db.Queries, pgx *pgxpool.Pool) *CommentRepository {
	return &CommentRepository{
		queries: queries,
		pgx:     pgx,
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

	var replyToString sql.NullString
	if replyToID != nil {
		replyToString = sql.NullString{
			String: replyToID.String(),
			Valid:  true,
		}
	}

	qtx, err := a.pgx.Begin(ctx)
	if err != nil {
		return "", nil, err
	}

	defer qtx.Rollback(ctx)

	qs := a.queries.WithTx(qtx)

	resultID, err := qs.InsertComment(ctx, db.InsertCommentParams{
		ID:        persist.GenerateID(),
		FeedEvent: feedEventString,
		Post:      postString,
		ActorID:   actorID,
		Reply:     replyToString,
		Comment:   comment,
	})
	if err != nil {
		return "", nil, err
	}

	ms := make([]db.Mention, len(mentions))

	for i, m := range mentions {
		var userID, communityID sql.NullString
		if m.UserID != "" {
			userID = sql.NullString{
				String: m.UserID.String(),
				Valid:  true,
			}
		} else if m.CommunityID != "" {
			communityID = sql.NullString{
				String: m.CommunityID.String(),
				Valid:  true,
			}
		}
		mid, err := qs.InsertMention(ctx, db.InsertMentionParams{
			ID:        persist.GenerateID(),
			CommentID: resultID,
			Start:     m.Start,
			Length:    m.Length,
			User:      userID,
			Community: communityID,
		})
		if err != nil {
			return "", nil, err
		}
		m.ID = mid
		ms[i] = m
	}

	err = qtx.Commit(ctx)
	if err != nil {
		return "", nil, err
	}

	return resultID, ms, nil
}
