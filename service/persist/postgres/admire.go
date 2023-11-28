package postgres

import (
	"context"

	"github.com/jackc/pgx/v4"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

// AdmireRepository represents an admire repository in the postgres database
type AdmireRepository struct {
	queries *db.Queries
}

// NewAdmireRepository creates a new postgres repository for interacting with admires
func NewAdmireRepository(queries *db.Queries) *AdmireRepository {
	return &AdmireRepository{queries: queries}
}

func (a *AdmireRepository) CreateFeedEventAdmire(ctx context.Context, newAdmireID, feedEventID, actorID persist.DBID) (persist.DBID, error) {
	return a.queries.CreateFeedEventAdmire(ctx, db.CreateFeedEventAdmireParams{ID: newAdmireID, FeedEventID: feedEventID, ActorID: actorID})
}

func (a *AdmireRepository) CreatePostAdmire(ctx context.Context, newAdmireID, postID, actorID persist.DBID) (persist.DBID, error) {
	return a.queries.CreatePostAdmire(ctx, db.CreatePostAdmireParams{ID: newAdmireID, PostID: postID, ActorID: actorID})
}

func (a *AdmireRepository) CreateTokenAdmire(ctx context.Context, newAdmireID, tokenID, actorID persist.DBID) (persist.DBID, error) {
	return a.queries.CreateTokenAdmire(ctx, db.CreateTokenAdmireParams{ID: newAdmireID, TokenID: tokenID, ActorID: actorID})
}

func (a *AdmireRepository) CreateCommentAdmire(ctx context.Context, newAdmireID, commentID, actorID persist.DBID) (persist.DBID, error) {
	return a.queries.CreateCommentAdmire(ctx, db.CreateCommentAdmireParams{ID: newAdmireID, CommentID: commentID, ActorID: actorID})
}

func mapResult(expected, actual persist.DBID, err, remapToMissingErr error) (persist.DBID, error) {
	// The subject to admire does not exist
	if err != nil {
		return "", err
	}
	// The admire already exists
	if expected != actual {
		return actual, persist.ErrAdmireAlreadyExists
	}
	return actual, nil
}

func (a *AdmireRepository) RemoveAdmire(ctx context.Context, admireID persist.DBID) error {
	return a.queries.DeleteAdmireByID(ctx, admireID)
}
