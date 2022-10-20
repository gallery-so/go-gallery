package publicapi

import (
	"context"
	"fmt"
	"time"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/task"

	gcptasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/model"
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

func (api ContractAPI) GetContractsDisplayedByUserID(ctx context.Context, userID persist.DBID) ([]db.Contract, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, err
	}

	contracts, err := api.loaders.ContractsDisplayedByUserID.Load(userID)
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

	err = api.multichainProvider.RefreshContract(ctx, persist.NewContractIdentifiers(contract.Address, contract.Chain))
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

	im, anim := contract.Chain.BaseKeywords()

	in := task.TokenProcessingContractTokensMessage{
		ContractID:        contractID,
		Imagekeywords:     im,
		Animationkeywords: anim,
	}
	return task.CreateTaskForContractOwnerProcessing(ctx, in, api.taskClient)
}

func (api ContractAPI) GetCommunityOwnersByContractAddress(ctx context.Context, contractAddress persist.ChainAddress, forceRefresh bool, before, after *string, first, last *int) ([]*model.TokenHolder, PageInfo, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"contractAddress": {contractAddress, "required"},
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	contract, err := api.loaders.ContractByChainAddress.Load(contractAddress)
	if err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params boolTimeIDPagingParams) ([]interface{}, error) {
		owners, err := api.loaders.OwnersByContractID.Load(db.GetOwnersByContractIdBatchPaginateParams{
			Contract:           contract.ID,
			Limit:              params.Limit,
			CurBeforeUniversal: params.CursorBeforeBool,
			CurAfterUniversal:  params.CursorAfterBool,
			CurBeforeTime:      params.CursorBeforeTime,
			CurBeforeID:        params.CursorBeforeID,
			CurAfterTime:       params.CursorAfterTime,
			CurAfterID:         params.CursorAfterID,
			PagingForward:      params.PagingForward,
		})

		if err != nil {
			return nil, err
		}

		results := make([]interface{}, len(owners))
		for i, owner := range owners {
			results[i] = owner
		}

		return results, nil
	}

	countFunc := func() (int, error) {
		total, err := api.queries.CountOwnersByContractId(ctx, contract.ID)
		return int(total), err
	}

	cursorFunc := func(i interface{}) (bool, time.Time, persist.DBID, error) {
		if user, ok := i.(db.User); ok {
			return user.Universal, user.CreatedAt, user.ID, nil
		}
		return false, time.Time{}, "", fmt.Errorf("interface{} is not a token")
	}

	paginator := boolTimeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	if err != nil {
		return nil, PageInfo{}, err
	}

	owners := make([]*model.TokenHolder, len(results))
	for i, result := range results {
		owner := result.(db.User)
		walletIDs := make([]persist.DBID, len(owner.Wallets))
		for j, wallet := range owner.Wallets {
			walletIDs[j] = wallet.ID
		}
		previewURLs, err := api.queries.GetPreviewURLsByContractIdAndUserId(ctx, db.GetPreviewURLsByContractIdAndUserIdParams{
			Contract:    contract.ID,
			OwnerUserID: owner.ID,
		})
		if err != nil {
			return nil, PageInfo{}, err
		}

		asStrings := make([]*string, len(previewURLs))
		for j, previewURL := range previewURLs {
			if asString, ok := previewURL.(string); ok {
				asStrings[j] = &asString
			}
		}

		owners[i] = &model.TokenHolder{
			HelperTokenHolderData: model.HelperTokenHolderData{
				UserId:    owner.ID,
				WalletIds: walletIDs,
			},
			DisplayName:   &owner.Username.String,
			Wallets:       nil, // handled by a dedicated resolver
			User:          nil, // handled by a dedicated resolver
			PreviewTokens: asStrings,
		}
	}

	return owners, pageInfo, nil
}
