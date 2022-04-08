package publicapi

import (
	"context"
	"github.com/mikeydub/go-gallery/db/sqlc"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/nft"
	"github.com/mikeydub/go-gallery/service/persist"
)

type NftAPI struct {
	repos     *persist.Repositories
	queries   *sqlc.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
}

func (api NftAPI) GetNftById(ctx context.Context, nftID persist.DBID) (*sqlc.Nft, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"nftID": {nftID, "required"},
	}); err != nil {
		return nil, err
	}

	nft, err := api.loaders.NftByNftId.Load(nftID)
	if err != nil {
		return nil, err
	}

	return &nft, nil
}

func (api NftAPI) GetNftsByCollectionId(ctx context.Context, collectionID persist.DBID) ([]sqlc.Nft, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"collectionID": {collectionID, "required"},
	}); err != nil {
		return nil, err
	}

	nfts, err := api.loaders.NftsByCollectionId.Load(collectionID)
	if err != nil {
		return nil, err
	}

	return nfts, nil
}

func (api NftAPI) GetNftsByOwnerAddress(ctx context.Context, ownerAddress persist.Address) ([]sqlc.Nft, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"ownerAddress": {ownerAddress, "required,eth_addr"},
	}); err != nil {
		return nil, err
	}

	nfts, err := api.loaders.NftsByOwnerAddress.Load(ownerAddress)
	if err != nil {
		return nil, err
	}

	return nfts, nil
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

	err = nft.RefreshOpenseaNFTs(ctx, userID, addresses, api.repos.NftRepository, api.repos.UserRepository)
	if err != nil {
		return err
	}

	api.loaders.ClearAllCaches()

	return nil
}
