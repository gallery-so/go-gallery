package publicapi

import (
	"context"

	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
)

type AdminAPI struct {
	repos     *postgres.Repositories
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
}

func (api *AdminAPI) AddRolesToUser(ctx context.Context, username string, roles []*persist.Role) (*db.User, error) {
	if err := validateFields(api.validator, validationMap{
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
	if err := validateFields(api.validator, validationMap{
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
