package publicapi

import (
	"context"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
)

type AdmireAPI struct {
	repos     *persist.Repositories
	queries   *sqlc.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
}

func (api AdmireAPI) GetAdmireByID(ctx context.Context, admireID persist.DBID) (*sqlc.Admire, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"admireID": {admireID, "required"},
	}); err != nil {
		return nil, err
	}

	admire, err := api.loaders.AdmireByAdmireId.Load(admireID)
	if err != nil {
		return nil, err
	}

	return &admire, nil
}

func (api AdmireAPI) GetAdmiresByFeedEventID(ctx context.Context, feedEventID persist.DBID) ([]sqlc.Admire, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
	}); err != nil {
		return nil, err
	}

	admires, err := api.loaders.AdmiresByFeedEventId.Load(feedEventID)
	if err != nil {
		return nil, err
	}

	return admires, nil
}

func (api AdmireAPI) AdmireFeedEvent(ctx context.Context, feedEventID persist.DBID, actorID persist.DBID) (persist.DBID, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"feedEventID": {feedEventID, "required"},
		"actorID":     {actorID, "required"},
	}); err != nil {
		return "", err
	}

	return api.repos.AdmireRepository.CreateAdmire(ctx, feedEventID, actorID)
}

func (api AdmireAPI) UnadmireFeedEvent(ctx context.Context, admireID persist.DBID) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"admireID": {admireID, "required"},
	}); err != nil {
		return err
	}

	return api.repos.AdmireRepository.RemoveAdmire(ctx, admireID)
}
