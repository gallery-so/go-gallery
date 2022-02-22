package persist

import (
	"context"
	"database/sql/driver"
	"fmt"
	"strings"
)

// FeatureFlag represents a feature flag in the database
type FeatureFlag struct {
	Version      NullInt32       `bson:"version"              json:"version"` // schema version for this model
	ID           DBID            `bson:"_id"                  json:"id" binding:"required"`
	CreationTime CreationTime    `bson:"created_at"        json:"created_at"`
	Deleted      NullBool        `bson:"deleted" json:"-"`
	LastUpdated  LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	RequiredToken       TokenIdentifiers `json:"required_token" bson:"required_token"`
	RequiredAmount      NullInt64        `json:"required_amount" bson:"required_amount"`
	TokenType           TokenType        `json:"token_type" bson:"token_type"`
	Name                NullString       `json:"name" bson:"name"`
	IsEnabled           NullBool         `json:"is_enabled" bson:"is_enabled"`
	AdminOnly           NullBool         `json:"admin_only" bson:"admin_only"`
	ForceEnabledUserIDs []DBID           `json:"force_enabled_users" bson:"force_enabled_users"`
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
	GetByRequiredTokens(context.Context, map[TokenIdentifiers]uint64) ([]FeatureFlag, error)
	GetByName(context.Context, string) (FeatureFlag, error)
	GetAll(context.Context) ([]FeatureFlag, error)
}

// NewTokenIdentifiers creates a new token identifiers
func NewTokenIdentifiers(pContractAddress Address, pTokenID TokenID) TokenIdentifiers {
	return TokenIdentifiers(pContractAddress.String() + "+" + pTokenID.String())
}

func (t TokenIdentifiers) String() string {
	if t.Valid() {
		return string(t)
	}
	return ""
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

// Value implements the driver.Valuer interface
func (t TokenIdentifiers) Value() (driver.Value, error) {
	return t.String(), nil
}

// Scan implements the database/sql Scanner interface for the TokenIdentifiers type
func (t *TokenIdentifiers) Scan(i interface{}) error {
	if i == nil {
		*t = TokenIdentifiers("")
		return nil
	}
	*t = TokenIdentifiers(i.(string))
	return nil
}

func (e ErrFeatureNotFoundByTokenIdentifiers) Error() string {
	return fmt.Sprintf("feature not found by token identifiers: %+v", e.TokenIdentifiers)
}

func (e ErrFeatureNotFoundByName) Error() string {
	return fmt.Sprintf("feature not found by name: %s", e.Name)
}
