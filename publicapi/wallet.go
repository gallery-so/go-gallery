package publicapi

import (
	"context"

	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/validate"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/multichain"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
)

type WalletAPI struct {
	repos              *postgres.Repositories
	queries            *db.Queries
	loaders            *dataloader.Loaders
	validator          *validator.Validate
	ethClient          *ethclient.Client
	multichainProvider *multichain.Provider
}

func (api WalletAPI) GetWalletByID(ctx context.Context, walletID persist.DBID) (*db.Wallet, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"walletID": {walletID, "required"},
	}); err != nil {
		return nil, err
	}

	address, err := api.loaders.WalletByWalletID.Load(walletID)
	if err != nil {
		return nil, err
	}

	return &address, nil
}

func (api WalletAPI) GetWalletByChainAddress(ctx context.Context, chainAddress persist.ChainAddress) (*db.Wallet, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
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

func (api WalletAPI) GetWalletsByUserID(ctx context.Context, userID persist.DBID) ([]db.Wallet, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
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
