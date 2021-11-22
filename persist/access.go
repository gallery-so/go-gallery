package persist

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Access represents a feature flag in the database
type Access struct {
	Version      int64              `bson:"version"              json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"                  json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at"        json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`
	LastUpdated  primitive.DateTime `bson:"last_updated,update_time" json:"last_updated"`

	UserID DBID `json:"user_id" bson:"user_id"`

	RequiredTokensOwned map[TokenIdentifiers]uint64 `json:"required_tokens_owned" bson:"required_tokens_owned"`
	IsAdmin             bool                        `json:"is_admin" bson:"is_admin"`
	MostRecentBlock     BlockNumber                 `json:"most_recent_block" bson:"most_recent_block"`
}

// ErrAccessNotFoundByUserID is an error type for when an access is not found by user id
type ErrAccessNotFoundByUserID struct {
	UserID DBID
}

// AccessRepository represents a repository for interacting with persisted access states
type AccessRepository interface {
	GetByUserID(context.Context, DBID) (*Access, error)
	HasRequiredTokens(context.Context, DBID, []TokenIdentifiers) (bool, error)
	UpdateRequiredTokensByUserID(context.Context, DBID, map[TokenIdentifiers]uint64, BlockNumber) error
}

func (e ErrAccessNotFoundByUserID) Error() string {
	return fmt.Sprintf("access not found for user id %s", e.UserID)
}
