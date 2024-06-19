package publicapi

import (
	"context"
	"database/sql"
	"time"

	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/validate"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/task"

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
	taskClient         *task.Client
}

func (api ContractAPI) GetContractByID(ctx context.Context, contractID persist.DBID) (*db.Contract, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"contractID": validate.WithTag(contractID, "required"),
	}); err != nil {
		return nil, err
	}

	contract, err := api.loaders.GetContractsByIDs.Load(contractID.String())
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

	contract, err := api.loaders.GetContractByChainAddressBatch.Load(db.GetContractByChainAddressBatchParams{
		Address: contractAddress.Address(),
		Chain:   contractAddress.Chain(),
	})
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

	queryFunc := func(params TimeIDPagingParams) ([]db.Contract, error) {
		return api.loaders.GetChildContractsByParentIDBatchPaginate.Load(db.GetChildContractsByParentIDBatchPaginateParams{
			ParentID:      contractID,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
			Limit:         params.Limit,
		})
	}

	cursorFunc := func(c db.Contract) (time.Time, persist.DBID, error) {
		return c.CreatedAt, c.ID, nil
	}

	paginator := TimeIDPaginator[db.Contract]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
	}

	return paginator.paginate(before, after, first, last)
}

func (api ContractAPI) GetContractCreatorByContractID(ctx context.Context, contractID persist.DBID) (db.ContractCreator, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"contractID": validate.WithTag(contractID, "required"),
	}); err != nil {
		return db.ContractCreator{}, err
	}

	return api.loaders.GetContractCreatorsByIds.Load(contractID.String())
}

func (api ContractAPI) GetContractsDisplayedByUserID(ctx context.Context, userID persist.DBID) ([]db.Contract, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
	}); err != nil {
		return nil, err
	}

	contracts, err := api.loaders.GetContractsDisplayedByUserIDBatch.Load(userID)
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

	contract, err := api.loaders.GetContractsByIDs.Load(contractID.String())
	if err != nil {
		return err
	}

	err = api.multichainProvider.RefreshContract(ctx, persist.NewContractIdentifiers(contract.Address, contract.Chain))
	if err != nil {
		return ErrTokenRefreshFailed{Message: err.Error()}
	}

	return nil
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

	contract, err := api.loaders.GetContractByChainAddressBatch.Load(db.GetContractByChainAddressBatchParams{
		Address: contractAddress.Address(),
		Chain:   contractAddress.Chain(),
	})
	if err != nil {
		return nil, PageInfo{}, err
	}

	boolFunc := func(params boolTimeIDPagingParams) ([]db.User, error) {
		return api.loaders.GetOwnersByContractIdBatchPaginate.Load(db.GetOwnersByContractIdBatchPaginateParams{
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
	}

	countFunc := func() (int, error) {

		total, err := api.queries.CountOwnersByContractId(ctx, db.CountOwnersByContractIdParams{
			ID:               contract.ID,
			GalleryUsersOnly: onlyGalleryUsers,
		})

		return int(total), err
	}

	boolCursorFunc := func(u db.User) (bool, time.Time, persist.DBID, error) {
		return u.Universal, u.CreatedAt, u.ID, nil
	}

	paginator := boolTimeIDPaginator[db.User]{
		QueryFunc:  boolFunc,
		CursorFunc: boolCursorFunc,
		CountFunc:  countFunc,
	}

	return paginator.paginate(before, after, first, last)
}

func (api ContractAPI) GetCommunityPostsByContractID(ctx context.Context, contractID persist.DBID, before, after *string, first, last *int) ([]db.Post, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"contractID": validate.WithTag(contractID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	timeFunc := func(params TimeIDPagingParams) ([]db.Post, error) {
		return api.loaders.PaginatePostsByContractID.Load(db.PaginatePostsByContractIDParams{
			ContractID:    contractID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})
	}

	countFunc := func() (int, error) {
		total, err := api.queries.CountPostsByContractID(ctx, contractID)
		return int(total), err
	}

	timeCursorFunc := func(p db.Post) (time.Time, persist.DBID, error) {
		return p.CreatedAt, p.ID, nil
	}

	paginator := TimeIDPaginator[db.Post]{
		QueryFunc:  timeFunc,
		CursorFunc: timeCursorFunc,
		CountFunc:  countFunc,
	}

	return paginator.paginate(before, after, first, last)
}

// ------ Temporary ------
func (api ContractAPI) GetCommunityPostsByContractIDAndProjectID(ctx context.Context, contractID persist.DBID, projectID int, before, after *string, first, last *int) ([]db.Post, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"contractID": validate.WithTag(contractID, "required"),
		"projectID":  validate.WithTag(projectID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	timeFunc := func(params TimeIDPagingParams) ([]db.Post, error) {
		return api.queries.PaginatePostsByContractIDAndProjectID(ctx, db.PaginatePostsByContractIDAndProjectIDParams{
			ContractID:    contractID,
			ProjectIDInt:  int32(projectID),
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})
	}

	countFunc := func() (int, error) {
		total, err := api.queries.CountPostsByContractID(ctx, contractID)
		return int(total), err
	}

	timeCursorFunc := func(p db.Post) (time.Time, persist.DBID, error) {
		return p.CreatedAt, p.ID, nil
	}

	paginator := TimeIDPaginator[db.Post]{
		QueryFunc:  timeFunc,
		CursorFunc: timeCursorFunc,
		CountFunc:  countFunc,
	}

	return paginator.paginate(before, after, first, last)
}

// End of temporary to-be-removed stuff

func (api ContractAPI) GetPreviewURLsByContractIDandUserID(ctx context.Context, userID, contractID persist.DBID) ([]string, error) {
	return api.queries.GetPreviewURLsByContractIdAndUserId(ctx, db.GetPreviewURLsByContractIdAndUserIdParams{
		ContractID: contractID,
		OwnerID:    userID,
	})
}
