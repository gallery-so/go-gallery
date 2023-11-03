package publicapi

import (
	"context"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/validate"

	gcptasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
)

type CommunityAPI struct {
	repos              *postgres.Repositories
	queries            *db.Queries
	loaders            *dataloader.Loaders
	validator          *validator.Validate
	ethClient          *ethclient.Client
	multichainProvider *multichain.Provider
	taskClient         *gcptasks.Client
}

func (api CommunityAPI) GetCommunityByKey(ctx context.Context, communityKey persist.CommunityKey) (*db.Community, error) {
	// Validate
	// TODO: Custom validator for persist.CommunityKey instead of checking individual fields here
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"communityKey.Type": validate.WithTag(communityKey.Type, "required"),
		"communityKey.Key":  validate.WithTag(communityKey.Key, "required"),
	}); err != nil {
		return nil, err
	}

	params := db.GetCommunityByKeyParams{
		Type:    int32(communityKey.Type),
		Subtype: communityKey.Subtype,
		Key:     communityKey.Key,
	}

	community, err := api.loaders.GetCommunityByKey.Load(params)
	if err != nil {
		return nil, err
	}

	return &community, nil
}
