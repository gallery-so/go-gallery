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
	return &AdmireRepository{queries: queries}
}

func (a *AdmireRepository) CreateFeedEventAdmire(ctx context.Context, feedEventID, actorID persist.DBID) (persist.DBID, error) {
	params := db.CreateFeedEventAdmireParams{ID: persist.GenerateID(), FeedEventID: feedEventID, ActorID: actorID}
	admireID, err := a.queries.CreateFeedEventAdmire(ctx, params)
	return mapResult(params.ID, admireID, err, persist.ErrFeedEventNotFoundByID{ID: feedEventID})
}

func (a *AdmireRepository) CreatePostAdmire(ctx context.Context, postID, actorID persist.DBID) (persist.DBID, error) {
	params := db.CreatePostAdmireParams{ID: persist.GenerateID(), PostID: postID, ActorID: actorID}
	admireID, err := a.queries.CreatePostAdmire(ctx, params)
	return mapResult(params.ID, admireID, err, persist.ErrPostNotFoundByID{ID: postID})
}

func (a *AdmireRepository) CreateTokenAdmire(ctx context.Context, tokenID, actorID persist.DBID) (persist.DBID, error) {
	params := db.CreateTokenAdmireParams{ID: persist.GenerateID(), TokenID: tokenID, ActorID: actorID}
	admireID, err := a.queries.CreateTokenAdmire(ctx, params)
	return mapResult(params.ID, admireID, err, persist.ErrTokenNotFoundByID{ID: tokenID})
}

func (a *AdmireRepository) CreateCommentAdmire(ctx context.Context, commentID, actorID persist.DBID) (persist.DBID, error) {
	params := db.CreateCommentAdmireParams{ID: persist.GenerateID(), CommentID: commentID, ActorID: actorID}
	admireID, err := a.queries.CreateCommentAdmire(ctx, params)
	return mapResult(params.ID, admireID, err, persist.ErrCommentNotFound{ID: commentID})
}

func mapResult(expectedID, actualID persist.DBID, err, missingErr error) (persist.DBID, error) {
	switch {
	case err != nil:
		return "", err
	// subject being admired does not exist
	case actualID == "":
		return "", missingErr
	// admire already exists
	case expectedID != actualID:
		return actualID, persist.ErrAdmireAlreadyExists
	default:
		return actualID, nil
	}
}

func (a *AdmireRepository) RemoveAdmire(ctx context.Context, admireID persist.DBID) error {
	return a.queries.DeleteAdmireByID(ctx, admireID)
}
