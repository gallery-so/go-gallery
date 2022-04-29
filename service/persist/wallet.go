package persist

import (
	"context"
	"database/sql/driver"
	"fmt"
	"io"

	"github.com/lib/pq"
)

// Wallet represents an address on any chain
type Wallet struct {
	ID           DBID            `json:"id"`
	Version      NullInt64       `json:"version"`
	CreationTime CreationTime    `json:"created_at"`
	Deleted      NullBool        `json:"-"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`

	Address    Address    `json:"address"`
	WalletType WalletType `json:"wallet_type"`
}

// WalletType is the type of wallet used to sign a message
type WalletType int

type WalletList []Wallet

const (
	// WalletTypeEOA represents an externally owned account (regular wallet address)
	WalletTypeEOA WalletType = iota
	// WalletTypeGnosis represents a smart contract gnosis safe
	WalletTypeGnosis
)

// WalletRepository represents a repository for interacting with persisted wallets
type WalletRepository interface {
	GetByAddressDetails(context.Context, AddressValue, Chain) (Wallet, error)
	GetByAddress(context.Context, DBID) (Wallet, error)
	Insert(context.Context, AddressValue, Chain, WalletType) (DBID, error)
}

func (l WalletList) Value() (driver.Value, error) {
	return pq.Array(l).Value()
}

// Scan implements the Scanner interface for the AddressList type
func (l *WalletList) Scan(value interface{}) error {
	return pq.Array(l).Scan(value)
}

// UnmarshalGQL implements the graphql.Unmarshaler interface
func (wa *WalletType) UnmarshalGQL(v interface{}) error {
	n, ok := v.(int)
	if !ok {
		return fmt.Errorf("Chain must be an int")
	}

	*wa = WalletType(n)
	return nil
}

// MarshalGQL implements the graphql.Marshaler interface
func (wa WalletType) MarshalGQL(w io.Writer) {
	w.Write([]byte{uint8(wa)})
}

// ErrWalletNotFoundByAddressDetails is an error type for when a wallet is not found by address and chain unique combination
type ErrWalletNotFoundByAddressDetails struct {
	Address AddressValue
	Chain   Chain
}

// ErrWalletNotFoundByAddress is an error type for when a wallet is not found by address's ID
type ErrWalletNotFoundByAddress struct {
	Address DBID
}

func (e ErrWalletNotFoundByAddressDetails) Error() string {
	return fmt.Sprintf("wallet not found by address details: %s | chain: %s", e.Address, e.Chain)
}

func (e ErrWalletNotFoundByAddress) Error() string {
	return fmt.Sprintf("wallet not found by address ID: %s", e.Address)
}
