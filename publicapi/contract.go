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

type ContractAPI struct {
	repos              *persist.Repositories
	queries            *sqlc.Queries
	loaders            *dataloader.Loaders
	validator          *validator.Validate
	ethClient          *ethclient.Client
	multichainProvider *multichain.Provider
}

func (api ContractAPI) GetContractByID(ctx context.Context, contractID persist.DBID) (*sqlc.Contract, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"contractID": {contractID, "required"},
	}); err != nil {
		return nil, err
	}

	contract, err := api.loaders.ContractByContractId.Load(contractID)
	if err != nil {
		return nil, err
	}

	return &contract, nil
}

func (api ContractAPI) GetContractByAddress(ctx context.Context, contractAddress persist.ChainAddress) (*sqlc.Contract, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"contractAddress": {contractAddress, "required"},
	}); err != nil {
		return nil, err
	}

	contract, err := api.loaders.ContractByChainAddress.Load(contractAddress)
	if err != nil {
		return nil, err
	}

	return &contract, nil
}

func (api ContractAPI) GetContractsByUserID(ctx context.Context, userID persist.DBID) ([]sqlc.Contract, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, err
	}

	contracts, err := api.loaders.ContractsByUserID.Load(userID)
	if err != nil {
		return nil, err
	}

	return contracts, nil
}

// RefreshContract refreshes the metadata for a given contract DBID
func (api ContractAPI) RefreshContract(ctx context.Context, contractID persist.DBID) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"contractID": {contractID, "required"},
	}); err != nil {
		return err
	}

	contract, err := api.loaders.ContractByContractId.Load(contractID)
	if err != nil {
		return err
	}

	err = api.multichainProvider.RefreshContract(ctx, persist.NewContractIdentifiers(contract.Address, persist.Chain(contract.Chain.Int32)))
	if err != nil {
		return ErrTokenRefreshFailed{Message: err.Error()}
	}

	api.loaders.ClearAllCaches()

	return nil

}

func (api ContractAPI) GetCommunityOwnersByContractAddress(ctx context.Context, contractAddress persist.ChainAddress, forceRefresh bool, onlyGalleryUsers bool) ([]multichain.TokenHolder, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"contractAddress": {contractAddress, "required"},
	}); err != nil {
		return nil, err
	}

	owners, err := api.multichainProvider.GetCommunityOwners(ctx, contractAddress, onlyGalleryUsers, forceRefresh)
	if err != nil {
		return nil, err
	}

	return owners, nil
}
