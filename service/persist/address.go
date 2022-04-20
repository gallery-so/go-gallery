package persist

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
)

// Address represents an address on any chain
type Address struct {
	ID           DBID            `json:"id"`
	Version      NullInt64       `json:"version"`
	CreationTime CreationTime    `json:"created_at"`
	Deleted      NullBool        `json:"-"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`

	Address NullString `json:"address"`
	Chain   Chain      `json:"chain"`
}

// AddressRepository represents a repository for interacting with persisted wallets
type AddressRepository interface {
	GetByDetails(context.Context, Address, Chain) (Address, error)
	GetByID(context.Context, DBID) (Address, error)
	Insert(context.Context, Address, Chain) error
}

// ErrAddressNotFoundByDetails is an error type for when a wallet is not found by address and chain unique combination
type ErrAddressNotFoundByDetails struct {
	Address Address
	Chain   Chain
}

func (a Address) String() string {
	return string(a.Address)
}

// ToHexAddress returns the address as a hex byte array (ethereum based)
func (a Address) ToHexAddress() common.Address {
	switch a.Chain {
	case ChainETH:
		return common.HexToAddress(a.String())
	default:
		return common.Address{}
	}
}
