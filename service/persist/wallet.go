package persist

import (
	"context"
	"database/sql/driver"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// Wallet represents an address on any chain
type Wallet struct {
	ID           DBID            `json:"id"`
	Version      NullInt64       `json:"version"`
	CreationTime CreationTime    `json:"created_at"`
	Deleted      NullBool        `json:"-"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`

	UserID  DBID    `json:"user_id"`
	Address Address `json:"address"`
	Chain   Chain   `json:"chain"`
}

type Address string

// WalletRepository represents a repository for interacting with persisted wallets
type WalletRepository interface {
	GetByAddress(context.Context, Address, Chain) (Wallet, error)
	GetByUserID(context.Context, DBID) ([]Wallet, error)
	Upsert(context.Context, Address, Chain, DBID) error
}

// ErrWalletNotFoundByAddress is an error type for when a wallet is not found by address and chain unique combination
type ErrWalletNotFoundByAddress struct {
	Address Address
	Chain   Chain
}

func (a Wallet) String() string {
	return string(a.Address)
}

// ToHexAddress returns the address as a hex byte array (ethereum based)
func (a Wallet) ToHexAddress() common.Address {
	switch a.Chain {
	case ChainETH:
		return common.HexToAddress(a.String())
	default:
		return common.Address{}
	}
}

func (n Address) String() string {
	return string(n)
}

// Value implements the database/sql driver Valuer interface for the NullString type
func (n Address) Value() (driver.Value, error) {
	if n.String() == "" {
		return "", nil
	}
	return strings.ToValidUTF8(strings.ReplaceAll(n.String(), "\\u0000", ""), ""), nil
}

// Scan implements the database/sql Scanner interface for the NullString type
func (n *Address) Scan(value interface{}) error {
	if value == nil {
		*n = Address("")
		return nil
	}
	*n = Address(value.(string))
	return nil
}

func (e ErrWalletNotFoundByAddress) Error() string {
	return fmt.Sprintf("wallet not found by address: %s | chain: %s", e.Address, e.Chain)
}

// WalletsToEthereumAddresses returns a list of ethereum addresses from a list of wallets
func WalletsToEthereumAddresses(wallets []Wallet) []EthereumAddress {
	addresses := make([]EthereumAddress, len(wallets))
	for i, wallet := range wallets {
		if wallet.Chain != ChainETH && wallet.Chain != "" {
			panic("unsupported chain")
		}
		addresses[i] = EthereumAddress(wallet.Address)
	}
	return addresses
}
