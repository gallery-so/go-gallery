package publicapi

import (
	"context"

	sqlc "github.com/mikeydub/go-gallery/db/sqlc/coregen"
	"github.com/mikeydub/go-gallery/service/multichain"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
)

type WalletAPI struct {
	repos              *persist.Repositories
	queries            *sqlc.Queries
	loaders            *dataloader.Loaders
	validator          *validator.Validate
	ethClient          *ethclient.Client
	multichainProvider *multichain.Provider
}

func (api WalletAPI) GetWalletByID(ctx context.Context, walletID persist.DBID) (*sqlc.Wallet, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"walletID": {walletID, "required"},
	}); err != nil {
		return nil, err
	}

	address, err := api.loaders.WalletByWalletId.Load(walletID)
	if err != nil {
		return nil, err
	}

	return &address, nil
}

func (api WalletAPI) GetWalletByChainAddress(ctx context.Context, chainAddress persist.ChainAddress) (*sqlc.Wallet, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"chainAddress": {chainAddress, "required"},
	}); err != nil {
		return nil, err
	}

	a, err := api.loaders.WalletByChainAddress.Load(chainAddress)
	if err != nil {
		return nil, err
	}

	return &a, nil
}

func (api WalletAPI) GetWalletsByUserID(ctx context.Context, userID persist.DBID) ([]sqlc.Wallet, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, err
	}

	a, err := api.loaders.WalletsByUserID.Load(userID)
	if err != nil {
		return nil, err
	}

	return a, nil
}
