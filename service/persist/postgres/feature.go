package postgres

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

// FeatureFlagRepository is a repository for feature flags
type FeatureFlagRepository struct {
	db *sql.DB
}

// NewFeatureFlagRepository returns a new FeatureFlagRepository
func NewFeatureFlagRepository(db *sql.DB) *FeatureFlagRepository {
	return &FeatureFlagRepository{db: db}
}

// GetByRequiredTokens returns all feature flags with the given required tokens and ensures that the amount is greater than or equal to the given amount
func (f *FeatureFlagRepository) GetByRequiredTokens(pCtx context.Context, pRequiredTokens map[persist.TokenIdentifiers]uint64) ([]persist.FeatureFlag, error) {
	keys := make([]persist.TokenIdentifiers, 0, len(pRequiredTokens))
	for k := range pRequiredTokens {
		keys = append(keys, k)
	}
	getFlagsSQLStr := `SELECT ID,VERSION,DELETED,LAST_UPDATED,CREATED_AT,REQUIRED_TOKEN,REQUIRED_AMOUNT,TOKEN_TYPE,NAME,IS_ENABLED,ADMIN_ONLY,FORCE_ENABLED_USER_IDS FROM features WHERE REQUIRED_TOKEN = ANY($1)`
	rows, err := f.db.QueryContext(pCtx, getFlagsSQLStr, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var flags []persist.FeatureFlag
	for rows.Next() {
		var flag persist.FeatureFlag
		err = rows.Scan(&flag.ID, &flag.Version, &flag.Deleted, &flag.LastUpdated, &flag.CreationTime, &flag.RequiredToken, &flag.RequiredAmount, &flag.TokenType, &flag.Name, &flag.IsEnabled, &flag.AdminOnly, pq.Array(&flag.ForceEnabledUserIDs))
		if err != nil {
			return nil, err
		}
		flags = append(flags, flag)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i, f := range flags {
		if f.RequiredAmount > pRequiredTokens[f.RequiredToken] {
			flags = append(flags[:i], flags[i+1:]...)
		}
	}

	return flags, nil

}

// GetByName returns a feature flag by name
func (f *FeatureFlagRepository) GetByName(pCtx context.Context, pName string) (persist.FeatureFlag, error) {
	getFlagSQLStr := `SELECT ID,VERSION,DELETED,LAST_UPDATED,CREATED_AT,REQUIRED_TOKEN,REQUIRED_AMOUNT,TOKEN_TYPE,NAME,IS_ENABLED,ADMIN_ONLY,FORCE_ENABLED_USER_IDS FROM features WHERE NAME = $1`
	var flag persist.FeatureFlag
	err := f.db.QueryRowContext(pCtx, getFlagSQLStr, pName).Scan(&flag.ID, &flag.Version, &flag.Deleted, &flag.LastUpdated, &flag.CreationTime, &flag.RequiredToken, &flag.RequiredAmount, &flag.TokenType, &flag.Name, &flag.IsEnabled, &flag.AdminOnly, pq.Array(&flag.ForceEnabledUserIDs))
	if err != nil {
		return flag, err
	}
	return flag, nil
}

// GetAll returns all feature flags
func (f *FeatureFlagRepository) GetAll(pCtx context.Context) ([]persist.FeatureFlag, error) {
	getFlagsSQLStr := `SELECT ID,VERSION,DELETED,LAST_UPDATED,CREATED_AT,REQUIRED_TOKEN,REQUIRED_AMOUNT,TOKEN_TYPE,NAME,IS_ENABLED,ADMIN_ONLY,FORCE_ENABLED_USER_IDS FROM features`
	rows, err := f.db.QueryContext(pCtx, getFlagsSQLStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var flags []persist.FeatureFlag
	for rows.Next() {
		var flag persist.FeatureFlag
		err = rows.Scan(&flag.ID, &flag.Version, &flag.Deleted, &flag.LastUpdated, &flag.CreationTime, &flag.RequiredToken, &flag.RequiredAmount, &flag.TokenType, &flag.Name, &flag.IsEnabled, &flag.AdminOnly, pq.Array(&flag.ForceEnabledUserIDs))
		if err != nil {
			return nil, err
		}
		flags = append(flags, flag)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return flags, nil
}
