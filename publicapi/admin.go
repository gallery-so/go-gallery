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
		"roles":    {roles, "required,dive,role"},
	}); err != nil {
		return nil, err
	}

	user, err := api.queries.GetUserByUsername(ctx, username)

	if err != nil {
		return nil, err
	}

	// Dedupe roles
	roleMap := map[persist.Role]bool{}
	for _, role := range roles {
		roleMap[*role] = true
	}
	for _, role := range user.Roles {
		roleMap[role] = true
	}
	newRoles := make(persist.RoleList, 0, len(roles)+len(user.Roles))
	for role := range roleMap {
		newRoles = append(newRoles, role)
	}

	user, err = api.queries.UpdateUserRoles(ctx, db.UpdateUserRolesParams{
		Roles: newRoles,
		ID:    user.ID,
	})

	return &user, err
}

func (api *AdminAPI) RemoveRolesFromUser(ctx context.Context, username string, roles []*persist.Role) (*db.User, error) {
	if err := validateFields(api.validator, validationMap{
		"username": {username, "required"},
		"roles":    {roles, "required,dive,role"},
	}); err != nil {
		return nil, err
	}

	user, err := api.queries.GetUserByUsername(ctx, username)

	if err != nil {
		return nil, err
	}

	roleMap := map[persist.Role]bool{}
	// Add existing roles to map
	for _, role := range user.Roles {
		roleMap[role] = true
	}
	// Remove unwanted roles from map
	for _, role := range roles {
		delete(roleMap, *role)
	}
	// Build new role array
	newRoles := make(persist.RoleList, 0, len(roleMap))
	for role := range roleMap {
		newRoles = append(newRoles, role)
	}

	user, err = api.queries.UpdateUserRoles(ctx, db.UpdateUserRolesParams{
		Roles: newRoles,
		ID:    user.ID,
	})

	return &user, err
}
