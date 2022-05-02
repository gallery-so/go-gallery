package publicapi

import (
	"context"

	"github.com/mikeydub/go-gallery/db/sqlc"
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
		"addressID": {walletID, "required"},
	}); err != nil {
		return nil, err
	}

	address, err := api.loaders.WalletByWalletId.Load(walletID)
	if err != nil {
		return nil, err
	}

	return &address, nil
}

func (api WalletAPI) GetWalletByDetails(ctx context.Context, address persist.AddressValue, chain persist.Chain) (*sqlc.Wallet, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"address": {address, "required"},
		"chain":   {chain, "required"},
	}); err != nil {
		return nil, err
	}

	a, err := api.loaders.WalletByAddressDetails.Load(persist.AddressDetails{AddressValue: address, Chain: chain})
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

	a, err := api.loaders.WalletByUserID.Load(userID)
	if err != nil {
		return nil, err
	}

	return a, nil
}
