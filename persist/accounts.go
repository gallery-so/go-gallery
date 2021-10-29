package persist

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

const accountCollName = "accounts"

// Address represents an Ethereum address
type Address string

// BlockNumber represents an Ethereum block number
type BlockNumber uint64

// Account represents an ethereum account in the database
type Account struct {
	Version      int64           `bson:"version"              json:"version"` // schema version for this model
	ID           DBID            `bson:"_id"                json:"id"`
	CreationTime CreationTime    `bson:"created_at"        json:"created_at"`
	Deleted      bool            `bson:"deleted" json:"-"`
	LastUpdated  LastUpdatedTime `bson:"last_updated,update_time" json:"last_updated"`

	Address         Address     `bson:"address" json:"address"`
	LastSyncedBlock BlockNumber `bson:"last_synced_block" json:"last_synced_block"`
}

// AccountRepository is the interface for interacting with the account persistence layer
type AccountRepository interface {
	GetByAddress(context.Context, Address) (*Account, error)
	UpsertByAddress(context.Context, Address, *Account) error
}

// ErrAccountNotFoundByAddress is an error that occurs when an account is not found by an address
type ErrAccountNotFoundByAddress struct {
	Address Address
}

func (e ErrAccountNotFoundByAddress) Error() string {
	return fmt.Sprintf("account not found by address: %s", e.Address)
}

func (a Address) String() string {
	return normalizeAddress(strings.ToLower(string(a)))
}

// Address returns the ethereum address byte array
func (a Address) Address() common.Address {
	return common.HexToAddress(a.String())
}

// Uint64 returns the ethereum block number as a uint64
func (b BlockNumber) Uint64() uint64 {
	return uint64(b)
}

// BigInt returns the ethereum block number as a big.Int
func (b BlockNumber) BigInt() *big.Int {
	return new(big.Int).SetUint64(b.Uint64())
}

func (b BlockNumber) String() string {
	return strings.ToLower(b.BigInt().String())
}

// Hex returns the ethereum block number as a hex string
func (b BlockNumber) Hex() string {
	return strings.ToLower(b.BigInt().Text(16))
}

func normalizeAddress(address string) string {
	withoutPrefix := strings.TrimPrefix(address, "0x")
	if len(withoutPrefix) < 40 {
		return ""
	}
	return "0x" + withoutPrefix[len(withoutPrefix)-40:]
}
