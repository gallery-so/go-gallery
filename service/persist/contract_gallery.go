package persist

import (
	"context"
)

// ContractGallery represents a smart contract in the database
type ContractGallery struct {
	Version      NullInt32       `json:"version"` // schema version for this model
	ID           DBID            `json:"id" binding:"required"`
	CreationTime CreationTime    `json:"created_at"`
	Deleted      NullBool        `json:"-"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`

	Chain          Chain           `json:"chain"`
	Address        Address         `json:"address"`
	Symbol         NullString      `json:"symbol"`
	Name           NullString      `json:"name"`
	CreatorAddress EthereumAddress `json:"creator_address"`
}

// ContractGalleryRepository represents a repository for interacting with persisted contracts
type ContractGalleryRepository interface {
	GetByAddress(context.Context, Address, Chain) (Contract, error)
	UpsertByAddress(context.Context, Address, Contract) error
	BulkUpsert(context.Context, []Contract) error
}
