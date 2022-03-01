package persist

import (
	"context"
	"fmt"
)

// Contract represents an ethereum contract in the database
type Contract struct {
	Version      NullInt32       `json:"version"` // schema version for this model
	ID           DBID            `json:"id" binding:"required"`
	CreationTime CreationTime    `json:"created_at"`
	Deleted      NullBool        `json:"-"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`

	Address Address    `json:"address"`
	Symbol  NullString `json:"symbol"`
	Name    NullString `json:"name"`

	LatestBlock BlockNumber `json:"latest_block"`
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
