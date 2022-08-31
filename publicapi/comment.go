package publicapi

import (
	"context"
	"errors"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
)

var ErrOnlyRemoveOwnComment = errors.New("only the actor who created the comment can remove it")

type CommentAPI struct {
	repos     *persist.Repositories
	queries   *sqlc.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
}

func (api CommentAPI) GetCommentByID(ctx context.Context, commentID persist.DBID) (*sqlc.Comment, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"commentID": {commentID, "required"},
	}); err != nil {
		return nil, err
	}

	comment, err := api.loaders.CommentById.Load(commentID)
	if err != nil {
		return nil, err
	}

	return &comment, nil
}

func (api CommentAPI) GetCommentsByFeedEventID(ctx context.Context, feedEventID persist.DBID) ([]sqlc.Comment, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
	}); err != nil {
		return nil, err
	}

	comments, err := api.loaders.CommentsByFeedEventId.Load(feedEventID)
	if err != nil {
		return nil, err
	}

	return comments, nil
}

func (api CommentAPI) CommentOnFeedEvent(ctx context.Context, feedEventID persist.DBID, actorID persist.DBID, replyToID *persist.DBID, comment string) (persist.DBID, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
		"actorID":     {actorID, "required"},
	}); err != nil {
		return "", err
	}

	return api.repos.CommentRepository.CreateComment(ctx, feedEventID, actorID, replyToID, comment)
}

func (api CommentAPI) RemoveComment(ctx context.Context, commentID persist.DBID, actorID persist.DBID) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"commentID": {commentID, "required"},
		"actorID":   {actorID, "required"},
	}); err != nil {
		return err
	}
	comment, err := api.GetCommentByID(ctx, commentID)
	if err != nil {
		return err
	}
	if comment.ActorID != actorID {
		return ErrOnlyRemoveOwnComment
	}

	return api.repos.CommentRepository.RemoveComment(ctx, commentID)
}
