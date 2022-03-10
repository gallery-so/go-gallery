package publicapi

import (
	"context"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/nft"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/pubsub"
)

type NftAPI struct {
	repos     *persist.Repositories
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
	pubsub    pubsub.PubSub
}

func (api NftAPI) RefreshOpenSeaNfts(ctx context.Context, addresses string) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"addresses": {addresses, "required"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	return nft.RefreshOpenseaNFTs(ctx, userID, addresses, api.repos.NftRepository, api.repos.UserRepository)
}
