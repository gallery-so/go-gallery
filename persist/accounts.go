package persist

import (
	"context"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const accountCollName = "accounts"

// Account represents an ethereum account in the database
type Account struct {
	Version      int64              `bson:"version"              json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"                  json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at"        json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`
	LastUpdated  primitive.DateTime `bson:"last_updated" json:"last_updated"`

	Address         string `bson:"address" json:"address"`
	LastSyncedBlock string `bson:"last_synced_block" json:"last_synced_block"`
}

// AccountRepository is the interface for interacting with the account persistence layer
type AccountRepository interface {
	GetByAddress(context.Context, string) (*Account, error)
	UpsertByAddress(context.Context, string, *Account) error
}
