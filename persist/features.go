package persist

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// FeatureFlag represents a feature flag in the database
type FeatureFlag struct {
	Version      int64              `bson:"version"              json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"                  json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at"        json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`
	LastUpdated  primitive.DateTime `bson:"last_updated,update_time" json:"last_updated"`

	RequiredToken       TokenIdentifiers `json:"token_identifiers" bson:"token_identifiers"`
	Name                string           `json:"name" bson:"name"`
	IsEnabled           bool             `json:"is_enabled" bson:"is_enabled"`
	AdminOnly           bool             `json:"admin_only" bson:"admin_only"`
	ForceEnabledUserIds []DBID           `json:"force_enabled_users" bson:"force_enabled_users"`
}

// TokenIdentifiers represents a unique identifier for a token
type TokenIdentifiers struct {
	TokenID         TokenID `json:"token_id" bson:"token_id"`
	ContractAddress Address `json:"contract_address" bson:"contract_address"`
}

// ErrFeatureNotFoundByTokenIdentifiers is an error type for when a feature is not found by token identifiers
type ErrFeatureNotFoundByTokenIdentifiers struct {
	TokenIdentifiers TokenIdentifiers
}

// ErrFeatureNotFoundByName is an error type for when a feature is not found by name
type ErrFeatureNotFoundByName struct {
	Name string
}

// FeatureFlagRepository represents a repository for interacting with persisted feature flags
type FeatureFlagRepository interface {
	GetByTokenIdentifiers(context.Context, TokenIdentifiers) (*FeatureFlag, error)
	GetByName(context.Context, string) (*FeatureFlag, error)
}

func (e ErrFeatureNotFoundByTokenIdentifiers) Error() string {
	return fmt.Sprintf("feature not found by token identifiers: %s", e.TokenIdentifiers)
}

func (e ErrFeatureNotFoundByName) Error() string {
	return fmt.Sprintf("feature not found by name: %s", e.Name)
}
