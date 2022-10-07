package publicapi

import (
	"context"
	"errors"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
)

var ErrOnlyRemoveOwnAdmire = errors.New("only the actor who created the admire can remove it")

type AdmireAPI struct {
	repos     *persist.Repositories
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
}

func (api AdmireAPI) GetAdmireByID(ctx context.Context, admireID persist.DBID) (*db.Admire, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"admireID": {admireID, "required"},
	}); err != nil {
		return nil, err
	}

	admire, err := api.loaders.AdmireByAdmireID.Load(admireID)
	if err != nil {
		return nil, err
	}

	return &admire, nil
}

func (api AdmireAPI) GetAdmiresByFeedEventID(ctx context.Context, feedEventID persist.DBID) ([]db.Admire, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
	}); err != nil {
		return nil, err
	}

	admires, err := api.loaders.AdmiresByFeedEventID.Load(feedEventID)
	if err != nil {
		return nil, err
	}

	return admires, nil
}

func (api AdmireAPI) AdmireFeedEvent(ctx context.Context, feedEventID persist.DBID) (persist.DBID, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
	}); err != nil {
		return "", err
	}

	actorID := For(ctx).User.GetLoggedInUserId(ctx)

	admireID, err := api.repos.AdmireRepository.CreateAdmire(ctx, feedEventID, actorID)
	if err != nil {
		return "", err
	}

	feedEvent, err := api.queries.GetEvent(ctx, feedEventID)
	if err != nil {
		return "", err
	}

	dispatchNotification(ctx, db.Notification{
		ActorID: actorID,
		OwnerID: feedEvent.UserID,
		Action:  persist.ActionAdmiredFeedEvent,
		Amount:  1,
		Data: persist.NotificationData{
			// TODO this has to concat with what is in the DB
			AdmirerIDs:  []persist.DBID{actorID},
			FeedEventID: feedEventID,
		},
	})
	return admireID, nil
}

func (api AdmireAPI) RemoveAdmire(ctx context.Context, admireID persist.DBID) (persist.DBID, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"admireID": {admireID, "required"},
	}); err != nil {
		return "", err
	}

	// will also fail if admire does not exist
	admire, err := api.GetAdmireByID(ctx, admireID)
	if err != nil {
		return "", err
	}
	if admire.ActorID != For(ctx).User.GetLoggedInUserId(ctx) {
		return "", ErrOnlyRemoveOwnAdmire
	}

	return admire.FeedEventID, api.repos.AdmireRepository.RemoveAdmire(ctx, admireID)
}
