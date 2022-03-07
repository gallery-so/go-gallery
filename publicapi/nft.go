package publicapi

import (
	"context"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/nft"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/pubsub"
)

type NftAPI struct {
	repos     *persist.Repositories
	loaders   *dataloader.Loaders
	ethClient *ethclient.Client
	pubsub    pubsub.PubSub
}

func (api NftAPI) RefreshOpenSeaNfts(ctx context.Context, addresses string) error {
	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	return nft.RefreshOpenseaNFTs(ctx, userID, addresses, api.repos.NftRepository, api.repos.UserRepository)
}
