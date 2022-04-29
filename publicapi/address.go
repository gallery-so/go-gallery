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

type AddressApi struct {
	repos              *persist.Repositories
	queries            *sqlc.Queries
	loaders            *dataloader.Loaders
	validator          *validator.Validate
	ethClient          *ethclient.Client
	multichainProvider *multichain.Provider
}

func (api AddressApi) GetAddressById(ctx context.Context, addressID persist.DBID) (*sqlc.Address, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"addressID": {addressID, "required"},
	}); err != nil {
		return nil, err
	}

	address, err := api.loaders.AddressByAddressId.Load(addressID)
	if err != nil {
		return nil, err
	}

	return &address, nil
}

func (api AddressApi) GetAddressByDetails(ctx context.Context, address persist.AddressValue, chain persist.Chain) (*sqlc.Address, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"address": {address, "required"},
		"chain":   {chain, "required"},
	}); err != nil {
		return nil, err
	}

	a, err := api.loaders.AddressByAddressDetails.Load(persist.AddressDetails{AddressValue: address, Chain: chain})
	if err != nil {
		return nil, err
	}

	return &a, nil
}
