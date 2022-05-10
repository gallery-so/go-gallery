package persist

import (
	"context"
	"database/sql/driver"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// Address represents an address on any chain
type Address struct {
	ID           DBID            `json:"id"`
	Version      NullInt64       `json:"version"`
	CreationTime CreationTime    `json:"created_at"`
	Deleted      NullBool        `json:"-"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`

	AddressValue AddressValue `json:"address"`
	Chain        Chain        `json:"chain"`
}

// AddressValue represents the value of an address
type AddressValue string

type AddressDetails struct {
	AddressValue AddressValue `json:"address"`
	Chain        Chain        `json:"chain"`
}

// AddressRepository represents a repository for interacting with persisted wallets
type AddressRepository interface {
	GetByDetails(context.Context, AddressValue, Chain) (Address, error)
	GetByID(context.Context, DBID) (Address, error)
	Insert(context.Context, AddressValue, Chain) error
}

// ErrAddressNotFoundByDetails is an error type for when a wallet is not found by address and chain unique combination
type ErrAddressNotFoundByDetails struct {
	Address AddressValue
	Chain   Chain
}

func (a Address) String() string {
	return string(a.AddressValue)
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

func (n AddressValue) String() string {
	return string(n)
}

// Value implements the database/sql driver Valuer interface for the NullString type
func (n AddressValue) Value() (driver.Value, error) {
	if n.String() == "" {
		return "", nil
	}
	return strings.ToValidUTF8(strings.ReplaceAll(n.String(), "\\u0000", ""), ""), nil
}

// Scan implements the database/sql Scanner interface for the NullString type
func (n *AddressValue) Scan(value interface{}) error {
	if value == nil {
		*n = AddressValue("")
		return nil
	}
	*n = AddressValue(value.(string))
	return nil
}

func (e ErrAddressNotFoundByDetails) Error() string {
	return fmt.Sprintf("address not found by details: %s, %d", e.Address, e.Chain)
}
