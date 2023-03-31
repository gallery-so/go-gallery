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

	Chain Chain `json:"chain"`

	Address      EthereumAddress `json:"address"`
	Symbol       NullString      `json:"symbol"`
	Name         NullString      `json:"name"`
	OwnerAddress EthereumAddress `json:"owner_address"`

	LatestBlock BlockNumber `json:"latest_block"`
}

// ContractUpdateInput is the input for updating contract metadata fields
type ContractUpdateInput struct {
	Symbol       NullString      `json:"symbol"`
	Name         NullString      `json:"name"`
	OwnerAddress EthereumAddress `json:"owner_address"`

	LatestBlock BlockNumber `json:"latest_block"`
}

// ContractRepository represents a repository for interacting with persisted contracts
type ContractRepository interface {
	GetByAddress(context.Context, EthereumAddress) (Contract, error)
	UpdateByAddress(context.Context, EthereumAddress, ContractUpdateInput) error
	UpsertByAddress(context.Context, EthereumAddress, Contract) error
	GetContractsOwnedByAddress(context.Context, EthereumAddress) ([]Contract, error)
	BulkUpsert(context.Context, []Contract) error
}

// ErrContractNotFoundByAddress is an error type for when a contract is not found by address
type ErrContractNotFoundByAddress struct {
	Address EthereumAddress
}

type ErrContractNotFoundByID struct {
	ID DBID
}

func (e ErrContractNotFoundByAddress) Error() string {
	return fmt.Sprintf("contract not found by address: %s", e.Address)
}

func (e ErrContractNotFoundByID) Error() string {
	return fmt.Sprintf("contract not found by ID: %s", e.ID)
}
