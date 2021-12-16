package persist

import (
	"context"
	"fmt"
)

// Contract represents an ethereum contract in the database
type Contract struct {
	Version      int64           `bson:"version"              json:"version"` // schema version for this model
	ID           DBID            `bson:"_id"                  json:"id" binding:"required"`
	CreationTime CreationTime    `bson:"created_at"        json:"created_at"`
	Deleted      bool            `bson:"deleted" json:"-"`
	LastUpdated  LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	Address Address `bson:"address" json:"address"`
	Symbol  string  `bson:"symbol" json:"symbol"`
	Name    string  `bson:"name" json:"name"`

	LatestBlock BlockNumber `bson:"latest_block" json:"latest_block"`
}

// ContractRepository represents a repository for interacting with persisted contracts
type ContractRepository interface {
	GetByAddress(context.Context, Address) (Contract, error)
	UpsertByAddress(context.Context, Address, Contract) error
	BulkUpsert(context.Context, []Contract) error
}

// ErrContractNotFoundByAddress is an error type for when a contract is not found by address
type ErrContractNotFoundByAddress struct {
	Address Address
}

func (e ErrContractNotFoundByAddress) Error() string {
	return fmt.Sprintf("contract not found by address: %s", e.Address)
}
