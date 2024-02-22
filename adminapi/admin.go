package adminapi

import (
	"context"
	"fmt"
	"github.com/mikeydub/go-gallery/service/auth/basicauth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/redis"

	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/user"
	"github.com/mikeydub/go-gallery/validate"
)

type AdminAPI struct {
	repos            *postgres.Repositories
	queries          *db.Queries
	authRefreshCache *redis.Cache
	validator        *validator.Validate
	multichain       *multichain.Provider
}

func NewAPI(repos *postgres.Repositories, queries *db.Queries, authRefreshCache *redis.Cache, validator *validator.Validate, mp *multichain.Provider) *AdminAPI {
	return &AdminAPI{repos, queries, authRefreshCache, validator, mp}
}

func (api *AdminAPI) AddRolesToUser(ctx context.Context, username string, roles []*persist.Role) (*db.User, error) {
	requireRetoolAuthorized(ctx)

	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"username": validate.WithTag(username, "required"),
		"roles":    validate.WithTag(roles, "required,unique,dive,role"),
	}); err != nil {
		return nil, err
	}

	user, err := api.queries.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	newRoles := make([]string, len(roles))
	for i, role := range roles {
		newRoles[i] = string(*role)
	}

	ids := make([]string, len(roles))
	for i := range roles {
		ids[i] = persist.GenerateID().String()
	}

	err = api.queries.AddUserRoles(ctx, db.AddUserRolesParams{
		UserID: user.ID,
		Ids:    ids,
		Roles:  newRoles,
	})

	if err != nil {
		return nil, err
	}

	err = auth.ForceAuthTokenRefresh(ctx, api.authRefreshCache, user.ID)
	if err != nil {
		logger.For(ctx).Errorf("error forcing auth token refresh for user %s: %s", user.ID, err)
	}

	return &user, err
}

func (api *AdminAPI) RemoveRolesFromUser(ctx context.Context, username string, roles []*persist.Role) (*db.User, error) {
	requireRetoolAuthorized(ctx)

	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"username": validate.WithTag(username, "required"),
		"roles":    validate.WithTag(roles, "required,unique,dive,role"),
	}); err != nil {
		return nil, err
	}

	user, err := api.queries.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	deleteRoles := make([]persist.Role, len(roles))
	for i, role := range roles {
		deleteRoles[i] = *role
	}

	err = api.queries.DeleteUserRoles(ctx, db.DeleteUserRolesParams{
		Roles:  deleteRoles,
		UserID: user.ID,
	})

	if err != nil {
		return nil, err
	}

	err = auth.ForceAuthTokenRefresh(ctx, api.authRefreshCache, user.ID)
	if err != nil {
		logger.For(ctx).Errorf("error forcing auth token refresh for user %s: %s", user.ID, err)
	}

	return &user, err
}

type authenticator struct {
	authMethod func(context.Context) (*auth.AuthResult, error)
}

func (a authenticator) GetDescription() string { return "" }
func (a authenticator) Authenticate(ctx context.Context) (*auth.AuthResult, error) {
	return a.authMethod(ctx)
}

func (api *AdminAPI) AddWalletToUserUnchecked(ctx context.Context, username string, chainAddress persist.ChainAddress, walletType persist.WalletType) error {
	requireRetoolAuthorized(ctx)

	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"username":     validate.WithTag(username, "required,username"),
		"chainAddress": validate.WithTag(chainAddress, "required"),
	}); err != nil {
		return err
	}

	u, err := api.repos.UserRepository.GetByUsername(ctx, username)
	if err != nil {
		return err
	}

	authMethod := func(ctx context.Context) (*auth.AuthResult, error) {
		err := auth.NonceRotate(ctx, chainAddress, api.repos.NonceRepository)
		if err != nil {
			return nil, err
		}

		authedAddress := auth.AuthenticatedAddress{
			ChainAddress: chainAddress,
			WalletID:     "",
			WalletType:   walletType,
		}

		return &auth.AuthResult{
			Addresses: []auth.AuthenticatedAddress{authedAddress},
		}, nil
	}

	return user.AddWalletToUser(ctx, u.ID, chainAddress, authenticator{authMethod}, api.repos.UserRepository, api.multichain)
}

func requireRetoolAuthorized(ctx context.Context) {
	requireBasicAuth(ctx, []basicauth.AuthTokenType{basicauth.AuthTokenTypeRetool})
}

func requireBasicAuth(ctx context.Context, allowedTypes []basicauth.AuthTokenType) {
	if !basicauth.AuthorizeHeaderForAllowedTypes(ctx, allowedTypes) {
		panic(fmt.Errorf("basic auth: not authorized for allowedTypes: %v", allowedTypes))
	}
}

func (api *AdminAPI) SetContractOverrideCreator(ctx context.Context, contractID persist.DBID, creatorUserID persist.DBID) error {
	requireRetoolAuthorized(ctx)

	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"contractID":    validate.WithTag(contractID, "required"),
		"creatorUserID": validate.WithTag(creatorUserID, "required"),
	}); err != nil {
		return err
	}

	params := db.SetContractOverrideCreatorParams{
		ContractID:    contractID,
		CreatorUserID: creatorUserID,
	}

	return api.queries.SetContractOverrideCreator(ctx, params)
}

func (api *AdminAPI) RemoveContractOverrideCreator(ctx context.Context, contractID persist.DBID) error {
	requireRetoolAuthorized(ctx)

	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"contractID": validate.WithTag(contractID, "required"),
	}); err != nil {
		return err
	}

	return api.queries.RemoveContractOverrideCreator(ctx, contractID)
}
