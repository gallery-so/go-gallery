package publicapi

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/validate"
	"time"
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

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {

		holders, err := api.loaders.PaginateHoldersByCommunityID.Load(db.PaginateHoldersByCommunityIDParams{
			CommunityID:   communityID,
			Limit:         sql.NullInt32{Int32: int32(params.Limit), Valid: true},
			CurBeforeTime: params.CursorBeforeTime,
			CurBeforeID:   params.CursorBeforeID,
			CurAfterTime:  params.CursorAfterTime,
			CurAfterID:    params.CursorAfterID,
			PagingForward: params.PagingForward,
		})

		if err != nil {
			return nil, err
		}

		results := make([]interface{}, len(holders))
		for i, owner := range holders {
			results[i] = owner
		}

		return results, nil
	}

	countFunc := func() (int, error) {
		total, err := api.queries.CountHoldersByCommunityID(ctx, communityID)
		return int(total), err
	}

	cursorFunc := func(i interface{}) (time.Time, persist.DBID, error) {
		if user, ok := i.(db.User); ok {
			return user.CreatedAt, user.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("interface{} is not a coredb.User")
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)
	if err != nil {
		return nil, PageInfo{}, err
	}

	holders := make([]db.User, len(results))
	for i, result := range results {
		if holder, ok := result.(db.User); ok {
			holders[i] = holder
		}
	}

	return holders, pageInfo, err
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

	timeFunc := func(params timeIDPagingParams) ([]interface{}, error) {

		posts, err := api.loaders.PaginatePostsByCommunityID.Load(db.PaginatePostsByCommunityIDParams{
			CommunityID:   communityID,
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
		for i, post := range posts {
			results[i] = post
		}

		return results, nil
	}

	countFunc := func() (int, error) {
		total, err := api.queries.CountPostsByCommunityID(ctx, communityID)
		return int(total), err
	}

	timeCursorFunc := func(i interface{}) (time.Time, persist.DBID, error) {
		if user, ok := i.(db.Post); ok {
			return user.CreatedAt, user.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("interface{} is not a post")
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

	queryFunc := func(params timeIDPagingParams) ([]interface{}, error) {

		rows, err := api.loaders.PaginateTokensByCommunityID.Load(db.PaginateTokensByCommunityIDParams{
			CommunityID:   communityID,
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

		results := make([]interface{}, len(rows))
		for i, r := range rows {
			results[i] = r.Token
		}

		return results, nil
	}

	countFunc := func() (int, error) {
		total, err := api.queries.CountTokensByCommunityID(ctx, communityID)
		return int(total), err
	}

	cursorFunc := func(i interface{}) (time.Time, persist.DBID, error) {
		if token, ok := i.(db.Token); ok {
			return token.CreatedAt, token.ID, nil
		}
		return time.Time{}, "", fmt.Errorf("interface{} is not a token")
	}

	paginator := timeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	if err != nil {
		return nil, PageInfo{}, err
	}

	tokens := make([]db.Token, len(results))
	for i, result := range results {
		if token, ok := result.(db.Token); ok {
			tokens[i] = token
		} else {
			return nil, PageInfo{}, fmt.Errorf("interface{} is not a token: %T", token)
		}
	}

	return tokens, pageInfo, nil
}
