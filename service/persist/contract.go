package persist

import (
	"context"
	"database/sql/driver"
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

	Address        EthereumAddress     `json:"address"`
	Symbol         NullString          `json:"symbol"`
	Name           NullString          `json:"name"`
	OwnerAddress   EthereumAddress     `json:"owner_address"`
	CreatorAddress EthereumAddress     `json:"creator_address"`
	OwnerMethod    ContractOwnerMethod `json:"owner_method"`

	LatestBlock BlockNumber `json:"latest_block"`
}

// ContractUpdateInput is the input for updating contract metadata fields
type ContractUpdateInput struct {
	Symbol         NullString      `json:"symbol"`
	Name           NullString      `json:"name"`
	OwnerAddress   EthereumAddress `json:"owner_address"`
	CreatorAddress EthereumAddress `json:"creator_address"`

	LatestBlock BlockNumber `json:"latest_block"`
}

type ContractOwnerMethod int

const (
	ContractOwnerMethodFailed ContractOwnerMethod = iota
	ContractOwnerMethodOwnable
	ContractOwnerMethodAlchemy
	ContractOwnerBinarySearch
)

// Value implements the driver.Valuer interface for the Chain type
func (c ContractOwnerMethod) Value() (driver.Value, error) {
	return c, nil
}

// Scan implements the sql.Scanner interface for the Chain type
func (c *ContractOwnerMethod) Scan(src interface{}) error {
	if src == nil {
		*c = ContractOwnerMethod(0)
		return nil
	}
	*c = ContractOwnerMethod(src.(int64))
	return nil
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
