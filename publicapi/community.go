package publicapi

import (
	"context"
	"database/sql"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

type CommunityAPI struct {
	repos              *postgres.Repositories
	queries            *db.Queries
	loaders            *dataloader.Loaders
	validator          *validator.Validate
	ethClient          *ethclient.Client
	multichainProvider *multichain.Provider
	taskClient         *task.Client
}

func (api CommunityAPI) GetCommunityByID(ctx context.Context, communityID persist.DBID) (*db.Community, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"communityID": validate.WithTag(communityID, "required"),
	}); err != nil {
		return nil, err
	}

	community, err := api.loaders.GetCommunityByIDBatch.Load(communityID)
	if err != nil {
		return nil, err
	}

	return &community, nil
}

func (api CommunityAPI) GetCommunityByKey(ctx context.Context, communityKey persist.CommunityKey) (*db.Community, error) {
	// Not much to validate here; different community types have different key requirements.
	params := db.GetCommunityByKeyParams{
		Type: int32(communityKey.Type),
		Key1: communityKey.Key1,
		Key2: communityKey.Key2,
		Key3: communityKey.Key3,
		Key4: communityKey.Key4,
	}

	community, err := api.loaders.GetCommunityByKey.Load(params)
	if err != nil {
		return nil, err
	}

	return &community, nil
}

func (api CommunityAPI) GetCreatorsByCommunityID(ctx context.Context, communityID persist.DBID) ([]db.GetCreatorsByCommunityIDRow, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"communityID": validate.WithTag(communityID, "required"),
	}); err != nil {
		return nil, err
	}

	return api.loaders.GetCreatorsByCommunityID.Load(communityID)
}

func (api CommunityAPI) PaginateHoldersByCommunityID(ctx context.Context, communityID persist.DBID, before, after *string, first, last *int) ([]db.User, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"communityID": validate.WithTag(communityID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	// Make sure the community exists
	_, err := api.GetCommunityByID(ctx, communityID)
	if err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]db.User, error) {
		return api.loaders.PaginateHoldersByCommunityID.Load(db.PaginateHoldersByCommunityIDParams{
			CommunityID:   communityID,
			Limit:         sql.NullInt32{Int32: int32(params.Limit), Valid: true},
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})
	}

	countFunc := func() (int, error) {
		total, err := api.queries.CountHoldersByCommunityID(ctx, communityID)
		return int(total), err
	}

	cursorFunc := func(u db.User) (time.Time, persist.DBID, error) {
		return u.CreatedAt, u.ID, nil
	}

	paginator := timeIDPaginator[db.User]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	return paginator.paginate(before, after, first, last)
}

func (api CommunityAPI) PaginatePostsByCommunityID(ctx context.Context, communityID persist.DBID, before, after *string, first, last *int) ([]db.Post, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"communityID": validate.WithTag(communityID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	timeFunc := func(params timeIDPagingParams) ([]db.Post, error) {
		return api.loaders.PaginatePostsByCommunityID.Load(db.PaginatePostsByCommunityIDParams{
			CommunityID:   communityID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})
	}

	countFunc := func() (int, error) {
		total, err := api.queries.CountPostsByCommunityID(ctx, communityID)
		return int(total), err
	}

	timeCursorFunc := func(p db.Post) (time.Time, persist.DBID, error) {
		return p.CreatedAt, p.ID, nil
	}

	paginator := timeIDPaginator[db.Post]{
		QueryFunc:  timeFunc,
		CursorFunc: timeCursorFunc,
		CountFunc:  countFunc,
	}

	return paginator.paginate(before, after, first, last)
}

func (api CommunityAPI) PaginateTokensByCommunityID(ctx context.Context, communityID persist.DBID, before, after *string, first, last *int) ([]db.Token, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"communityID": validate.WithTag(communityID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params timeIDPagingParams) ([]db.Token, error) {
		results, err := api.loaders.PaginateTokensByCommunityID.Load(db.PaginateTokensByCommunityIDParams{
			CommunityID:   communityID,
			Limit:         params.Limit,
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})
		return util.MapWithoutError(results, func(r db.PaginateTokensByCommunityIDRow) db.Token { return r.Token }), err
	}

	countFunc := func() (int, error) {
		total, err := api.queries.CountTokensByCommunityID(ctx, communityID)
		return int(total), err
	}

	cursorFunc := func(t db.Token) (time.Time, persist.DBID, error) {
		return t.CreatedAt, t.ID, nil
	}

	paginator := timeIDPaginator[db.Token]{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	return paginator.paginate(before, after, first, last)
}
