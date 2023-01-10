package admin

import (
	"context"

	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/publicapi/inputcheck"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/user"
)

type AdminAPI struct {
	repos     *postgres.Repositories
	queries   *db.Queries
	validator *validator.Validate
}

func NewAPI(repos *postgres.Repositories, queries *db.Queries, validator *validator.Validate) *AdminAPI {
	return &AdminAPI{repos, queries, validator}
}

func (api *AdminAPI) AddRolesToUser(ctx context.Context, username string, roles []*persist.Role) (*db.User, error) {
	requireRetoolAuthorized(ctx)

	if err := inputcheck.ValidateFields(api.validator, inputcheck.ValidationMap{
		"username": {username, "required"},
		"roles":    {roles, "required,unique,dive,role"},
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

	return &user, err
}

func (api *AdminAPI) RemoveRolesFromUser(ctx context.Context, username string, roles []*persist.Role) (*db.User, error) {
	requireRetoolAuthorized(ctx)

	if err := inputcheck.ValidateFields(api.validator, inputcheck.ValidationMap{
		"username": {username, "required"},
		"roles":    {roles, "required,unique,dive,role"},
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

	if err := inputcheck.ValidateFields(api.validator, inputcheck.ValidationMap{
		"username":     {username, "required,username"},
		"chainAddress": {chainAddress, "required"},
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

	return user.AddWalletToUser(ctx, u.ID, chainAddress, authenticator{authMethod}, api.repos.UserRepository, api.repos.WalletRepository)
}

func requireRetoolAuthorized(ctx context.Context) {
	if err := auth.RetoolAuthorized(ctx); err != nil {
		panic(err)
	}
}
