package publicapi

import (
	"context"
	"errors"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
<<<<<<< HEAD
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
=======
	sqlc "github.com/mikeydub/go-gallery/db/sqlc/coregen"
>>>>>>> a1c2c79 (Fix README commands)
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
)

var ErrOnlyRemoveOwnComment = errors.New("only the actor who created the comment can remove it")

type CommentAPI struct {
	repos     *persist.Repositories
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
}

func (api CommentAPI) GetCommentByID(ctx context.Context, commentID persist.DBID) (*db.Comment, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"commentID": {commentID, "required"},
	}); err != nil {
		return nil, err
	}

	comment, err := api.loaders.CommentByCommentId.Load(commentID)
	if err != nil {
		return nil, err
	}

	return &comment, nil
}

func (api CommentAPI) GetCommentsByFeedEventID(ctx context.Context, feedEventID persist.DBID) ([]db.Comment, error) {
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

func (api CommentAPI) CommentOnFeedEvent(ctx context.Context, feedEventID persist.DBID, replyToID *persist.DBID, comment string) (persist.DBID, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
	}); err != nil {
		return "", err
	}

	return api.repos.CommentRepository.CreateComment(ctx, feedEventID, For(ctx).User.GetLoggedInUserId(ctx), replyToID, comment)
}

func (api CommentAPI) RemoveComment(ctx context.Context, commentID persist.DBID) (persist.DBID, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"commentID": {commentID, "required"},
	}); err != nil {
		return "", err
	}
	comment, err := api.GetCommentByID(ctx, commentID)
	if err != nil {
		return "", err
	}
	if comment.ActorID != For(ctx).User.GetLoggedInUserId(ctx) {
		return "", ErrOnlyRemoveOwnComment
	}

	return comment.FeedEventID, api.repos.CommentRepository.RemoveComment(ctx, commentID)
}
