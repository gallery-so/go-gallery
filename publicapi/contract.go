package publicapi

import (
	"context"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/task"

	gcptasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
)

type ContractAPI struct {
	repos              *persist.Repositories
	queries            *db.Queries
	loaders            *dataloader.Loaders
	validator          *validator.Validate
	ethClient          *ethclient.Client
	multichainProvider *multichain.Provider
	taskClient         *gcptasks.Client
}

func (api ContractAPI) GetContractByID(ctx context.Context, contractID persist.DBID) (*db.Contract, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"contractID": {contractID, "required"},
	}); err != nil {
		return nil, err
	}

	contract, err := api.loaders.ContractByContractID.Load(contractID)
	if err != nil {
		return nil, err
	}

	return &contract, nil
}

func (api ContractAPI) GetContractByAddress(ctx context.Context, contractAddress persist.ChainAddress) (*db.Contract, error) {
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

func (api ContractAPI) GetContractsByUserID(ctx context.Context, userID persist.DBID) ([]db.Contract, error) {
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

	contract, err := api.loaders.ContractByContractID.Load(contractID)
	if err != nil {
		return err
	}

	err = api.multichainProvider.RefreshContract(ctx, persist.NewContractIdentifiers(contract.Address, persist.Chain(contract.Chain.Int32)))
	if err != nil {
		return ErrTokenRefreshFailed{Message: err.Error()}
	}

	return nil

}

func (api ContractAPI) RefreshOwnersAsync(ctx context.Context, contractID persist.DBID) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"contractID": {contractID, "required"},
	}); err != nil {
		return err
	}

	contract, err := api.loaders.ContractByContractID.Load(contractID)
	if err != nil {
		return err
	}

	im, anim := persist.Chain(contract.Chain.Int32).BaseKeywords()

	in := task.TokenProcessingContractTokensMessage{
		ContractID:        contractID,
		Imagekeywords:     im,
		Animationkeywords: anim,
	}
	return task.CreateTaskForContractOwnerProcessing(ctx, in, api.taskClient)
}

func (api ContractAPI) GetCommunityOwnersByContractAddress(ctx context.Context, contractAddress persist.ChainAddress, forceRefresh bool, limit, offset int) ([]multichain.TokenHolder, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"contractAddress": {contractAddress, "required"},
	}); err != nil {
		return nil, err
	}

	owners, err := api.multichainProvider.GetCommunityOwners(ctx, contractAddress, forceRefresh, limit, offset)
	if err != nil {
		return nil, err
	}

	return owners, nil
}
