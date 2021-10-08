package persist

import (
	"context"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Contract represents an ethereum contract in the database
type Contract struct {
	Version      int64              `bson:"version"              json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"                  json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at"        json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`
	LastUpdated  primitive.DateTime `bson:"last_updated" json:"last_updated"`

	Address string `bson:"address" json:"address"`
	Symbol  string `bson:"symbol" json:"symbol"`
	Name    string `bson:"name" json:"name"`

	LatestBlock uint64 `bson:"latest_block" json:"latest_block"`
}

// ContractRepository represents a repository for interacting with persisted contracts
type ContractRepository interface {
	GetByAddress(context.Context, string) ([]*Contract, error)
	UpsertByAddress(pCtx context.Context, pAddress string, pUpsert *Contract) error
}
