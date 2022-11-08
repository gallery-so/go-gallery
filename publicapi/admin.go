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

func (api *AdminAPI) AddRolesToUser(ctx context.Context, userID persist.DBID, roles []string) (*db.User, error) {
	if err := validateFields(api.validator, validationMap{
		"roles": {roles, "required,dive,role"},
	}); err != nil {
		return nil, err
	}

	user, err := api.queries.GetUserById(ctx, userID)

	if err != nil {
		return nil, err
	}

	// Start dedupe roles
	roleMap := map[string]bool{}
	for _, role := range roles {
		roleMap[role] = true
	}
	for _, role := range user.Roles {
		roleMap[role] = true
	}
	newRoles := make([]string, 0, len(roles)+len(user.Roles))
	for role := range roleMap {
		newRoles = append(newRoles, role)
	}
	// End dedupe roles

	user, err = api.queries.UpdateUserRoles(ctx, db.UpdateUserRolesParams{
		Roles: newRoles,
		ID:    userID,
	})

	return &user, err
}

func (api *AdminAPI) RemoveRolesFromUser(ctx context.Context, userID persist.DBID, roles []string) (*db.User, error) {
	if err := validateFields(api.validator, validationMap{
		"roles": {roles, "required,dive,role"},
	}); err != nil {
		return nil, err
	}

	user, err := api.queries.GetUserById(ctx, userID)

	if err != nil {
		return nil, err
	}

	roleMap := map[string]bool{}
	// Add all roles to map
	for _, role := range user.Roles {
		roleMap[role] = true
	}
	// Remove unwanted roles from map
	for _, role := range roles {
		delete(roleMap, role)
	}
	// Build new role array
	newRoles := make([]string, 0, len(roles)+len(user.Roles))
	for role := range roleMap {
		newRoles = append(newRoles, role)
	}

	user, err = api.queries.UpdateUserRoles(ctx, db.UpdateUserRolesParams{
		Roles: newRoles,
		ID:    userID,
	})

	return &user, err
}
