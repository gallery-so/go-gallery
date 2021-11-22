package persist

import (
	"context"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// FeatureFlag represents a feature flag in the database
type FeatureFlag struct {
	Version      int64              `bson:"version"              json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"                  json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at"        json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`
	LastUpdated  primitive.DateTime `bson:"last_updated,update_time" json:"last_updated"`

	RequiredToken       TokenIdentifiers `json:"required_token" bson:"required_token"`
	RequiredAmount      uint64           `json:"required_amount" bson:"required_amount"`
	TokenType           TokenType        `json:"token_type" bson:"token_type"`
	Name                string           `json:"name" bson:"name"`
	IsEnabled           bool             `json:"is_enabled" bson:"is_enabled"`
	AdminOnly           bool             `json:"admin_only" bson:"admin_only"`
	ForceEnabledUserIds []DBID           `json:"force_enabled_users" bson:"force_enabled_users"`
}

// TokenIdentifiers represents a unique identifier for a token
type TokenIdentifiers string

// ErrFeatureNotFoundByTokenIdentifiers is an error type for when a feature is not found by token identifiers
type ErrFeatureNotFoundByTokenIdentifiers struct {
	TokenIdentifiers []TokenIdentifiers
}

// ErrFeatureNotFoundByName is an error type for when a feature is not found by name
type ErrFeatureNotFoundByName struct {
	Name string
}

// FeatureFlagRepository represents a repository for interacting with persisted feature flags
type FeatureFlagRepository interface {
	GetByRequiredTokens(context.Context, map[TokenIdentifiers]uint64) ([]*FeatureFlag, error)
	GetByName(context.Context, string) (*FeatureFlag, error)
	GetAll(context.Context) ([]*FeatureFlag, error)
}

// NewTokenIdentifiers creates a new token identifiers
func NewTokenIdentifiers(pContractAddress Address, pTokenID TokenID) TokenIdentifiers {
	return TokenIdentifiers(pContractAddress.String() + "+" + pTokenID.String())
}

func (t TokenIdentifiers) String() string {
	if t.Valid() {
		return string(t)
	}
	panic("invalid token identifiers")
}

// Valid returns true if the token identifiers are valid
func (t TokenIdentifiers) Valid() bool {
	return len(strings.Split(string(t), "+")) == 2
}

// GetParts returns the parts of the token identifiers
func (t TokenIdentifiers) GetParts() (Address, TokenID) {
	parts := strings.Split(t.String(), "+")

	return Address(parts[0]), TokenID(parts[1])
}

func (e ErrFeatureNotFoundByTokenIdentifiers) Error() string {
	return fmt.Sprintf("feature not found by token identifiers: %+v", e.TokenIdentifiers)
}

func (e ErrFeatureNotFoundByName) Error() string {
	return fmt.Sprintf("feature not found by name: %s", e.Name)
}
