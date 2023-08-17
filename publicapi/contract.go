package publicapi

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/validate"

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
	repos              *postgres.Repositories
	queries            *db.Queries
	loaders            *dataloader.Loaders
	validator          *validator.Validate
	ethClient          *ethclient.Client
	multichainProvider *multichain.Provider
	taskClient         *gcptasks.Client
}

func (api ContractAPI) GetContractByID(ctx context.Context, contractID persist.DBID) (*db.Contract, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"contractID": validate.WithTag(contractID, "required"),
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
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"contractAddress": validate.WithTag(contractAddress, "required"),
	}); err != nil {
		return nil, err
	}

	contract, err := api.loaders.ContractByChainAddress.Load(contractAddress)
	if err != nil {
		return nil, err
	}

	return &contract, nil
}

func (api ContractAPI) GetChildContractsByParentID(ctx context.Context, contractID persist.DBID, before, after *string, first, last *int) ([]db.Contract, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"contractID": validate.WithTag(contractID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]any, error) {
		queryParams := db.GetChildContractsByParentIDBatchPaginateParams{
			ParentID:      contractID,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
			Limit:         params.Limit,
		}

		keys, err := api.loaders.ContractsLoaderByParentID.Load(queryParams)
		if err != nil {
			return nil, err
		}

		results := make([]any, len(keys))
		for i, key := range keys {
			results[i] = key
		}

		return results, nil
	}

	cursorFunc := func(i any) (time.Time, persist.DBID, error) {
		if row, ok := i.(db.Contract); ok {
			return row.CreatedAt, row.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("node is not a db.Contract")
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	contracts := make([]db.Contract, len(results))
	for i, result := range results {
		if contract, ok := result.(db.Contract); ok {
			contracts[i] = contract
		}
	}

	return contracts, pageInfo, err
}

func (api ContractAPI) GetContractCreatorByContractID(ctx context.Context, contractID persist.DBID) (db.ContractCreator, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"contractID": validate.WithTag(contractID, "required"),
	}); err != nil {
		return db.ContractCreator{}, err
	}

	return api.loaders.ContractCreatorByContractID.Load(contractID)
}

func (api ContractAPI) GetContractsDisplayedByUserID(ctx context.Context, userID persist.DBID) ([]db.Contract, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
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
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"contractID": validate.WithTag(contractID, "required"),
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

func (api ContractAPI) RefreshOwnersAsync(ctx context.Context, contractID persist.DBID, forceRefresh bool) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"contractID": validate.WithTag(contractID, "required"),
	}); err != nil {
		return err
	}

	in := task.TokenProcessingContractTokensMessage{
		ContractID:   contractID,
		ForceRefresh: forceRefresh,
	}
	return task.CreateTaskForContractOwnerProcessing(ctx, in, api.taskClient)
}

func (api ContractAPI) GetCommunityOwnersByContractAddress(ctx context.Context, contractAddress persist.ChainAddress, before, after *string, first, last *int, onlyGalleryUsers bool) ([]db.User, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"contractAddress": validate.WithTag(contractAddress, "required"),
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

	boolFunc := func(params boolTimeIDPagingParams) ([]interface{}, error) {

		owners, err := api.loaders.OwnersByContractID.Load(db.GetOwnersByContractIdBatchPaginateParams{
			ID:                 contract.ID,
			Limit:              sql.NullInt32{Int32: int32(params.Limit), Valid: true},
			GalleryUsersOnly:   onlyGalleryUsers,
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

		total, err := api.queries.CountOwnersByContractId(ctx, db.CountOwnersByContractIdParams{
			ID:               contract.ID,
			GalleryUsersOnly: onlyGalleryUsers,
		})

		return int(total), err
	}

	boolCursorFunc := func(i interface{}) (bool, time.Time, persist.DBID, error) {
		if user, ok := i.(db.User); ok {
			return user.Universal, user.CreatedAt, user.ID, nil
		}
		return false, time.Time{}, "", fmt.Errorf("interface{} is not a token")
	}

	paginator := boolTimeIDPaginator{
		QueryFunc:  boolFunc,
		CursorFunc: boolCursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)
	if err != nil {
		return nil, PageInfo{}, err
	}

	owners := make([]db.User, len(results))
	for i, result := range results {
		if owner, ok := result.(db.User); ok {
			owners[i] = owner
		}
	}

	return owners, pageInfo, err
}

func (api ContractAPI) GetCommunityPostsByContractID(ctx context.Context, contractID persist.DBID, before, after *string, first, last *int) ([]db.Post, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"contractAddress": validate.WithTag(contractID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	timeFunc := func(params timeIDPagingParams) ([]interface{}, error) {

		posts, err := api.loaders.PostsPaginatedByContractID.Load(db.PaginatePostsByContractIDParams{
			ContractID:    contractID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})

		if err != nil {
			return nil, err
		}

		results := make([]interface{}, len(posts))
		for i, owner := range posts {
			results[i] = owner
		}

		return results, nil
	}

	countFunc := func() (int, error) {
		total, err := api.queries.CountPostsByContractID(ctx, contractID)
		return int(total), err
	}

	timeCursorFunc := func(i interface{}) (time.Time, persist.DBID, error) {
		if user, ok := i.(db.Post); ok {
			return user.CreatedAt, user.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("interface{} is not a token")
	}

	paginator := timeIDPaginator{
		QueryFunc:  timeFunc,
		CursorFunc: timeCursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)
	if err != nil {
		return nil, PageInfo{}, err
	}

	posts := make([]db.Post, len(results))
	for i, result := range results {
		if post, ok := result.(db.Post); ok {
			posts[i] = post
		}
	}

	return posts, pageInfo, err
}

func (api ContractAPI) GetPreviewURLsByContractIDandUserID(ctx context.Context, userID, contractID persist.DBID) ([]string, error) {
	return api.queries.GetPreviewURLsByContractIdAndUserId(ctx, db.GetPreviewURLsByContractIdAndUserIdParams{
		Contract:    contractID,
		OwnerUserID: userID,
	})
}
