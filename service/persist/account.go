package persist

import (
	"context"
	"fmt"
)

const accountCollName = "accounts"

// Account represents an ethereum account in the database
type Account struct {
	Version      int64           `bson:"version"              json:"version"` // schema version for this model
	ID           DBID            `bson:"_id"                json:"id"`
	CreationTime CreationTime    `bson:"created_at"        json:"created_at"`
	Deleted      bool            `bson:"deleted" json:"-"`
	LastUpdated  LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	Address         Address     `bson:"address" json:"address"`
	LastSyncedBlock BlockNumber `bson:"last_synced_block" json:"last_synced_block"`
}

// AccountRepository is the interface for interacting with the account persistence layer
type AccountRepository interface {
	GetByAddress(context.Context, Address) (Account, error)
	UpsertByAddress(context.Context, Address, Account) error
}

// ErrAccountNotFoundByAddress is an error that occurs when an account is not found by an address
type ErrAccountNotFoundByAddress struct {
	Address Address
}

func (e ErrAccountNotFoundByAddress) Error() string {
	return fmt.Sprintf("account not found by address: %s", e.Address)
}
