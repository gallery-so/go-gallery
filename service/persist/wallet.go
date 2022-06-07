package persist

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/lib/pq"
)

var errWalletValueNoID = fmt.Errorf("wallet value has no ID")

// Wallet represents an address on any chain
type Wallet struct {
	ID           DBID            `json:"id"`
	Version      NullInt64       `json:"version"`
	CreationTime CreationTime    `json:"created_at"`
	Deleted      NullBool        `json:"-"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`

	Address    Address    `json:"address"`
	Chain      Chain      `json:"chain"`
	WalletType WalletType `json:"wallet_type"`
}

// WalletType is the type of wallet used to sign a message
type WalletType int

type WalletList []Wallet

// Address represents the value of an address
type Address string

//type ChainAddress struct {
//	Address Address `json:"address"`
//	Chain   Chain   `json:"chain"`
//}
//
//func (c ChainAddress) String() string {
//	return fmt.Sprintf("%d:%s", c.Chain, c.Address)
//}

type ChainAddress struct {
	addressSet bool
	chainSet   bool
	address    Address
	chain      Chain
}

// IsGalleryUserOrAddress is an empty function that satisfies the gqlgen IsGalleryUserOrAddress interface,
// allowing ChainAddress to be used in GraphQL resolvers that return the GalleryUserOrAddress type.
func (c *ChainAddress) IsGalleryUserOrAddress() {}

func NewChainAddress(address Address, chain Chain) ChainAddress {
	ca := ChainAddress{
		addressSet: true,
		chainSet:   true,
		address:    address,
		chain:      chain,
	}

	ca.updateCasing()
	return ca
}

func (c *ChainAddress) Address() Address {
	return c.address
}

func (c *ChainAddress) Chain() Chain {
	return c.chain
}

func (c *ChainAddress) updateCasing() {
	switch c.chain {
	// TODO: Add an IsCaseSensitive to the Chain type?
	case ChainETH:
		c.address = Address(strings.ToLower(c.address.String()))
	}
}

// GQLSetAddressFromResolver will be called automatically from the required gqlgen resolver and should
// never be called manually. To set a ChainAddress's fields, use NewChainAddress.
func (c *ChainAddress) GQLSetAddressFromResolver(address Address) error {
	if c.addressSet {
		return errors.New("ChainAddress.address may only be set once")
	}

	c.address = address
	c.addressSet = true

	if c.chainSet {
		c.updateCasing()
	}

	return nil
}

// GQLSetChainFromResolver will be called automatically from the required gqlgen resolver and should
// never be called manually. To set a ChainAddress's fields, use NewChainAddress.
func (c *ChainAddress) GQLSetChainFromResolver(chain Chain) error {
	if c.chainSet {
		return errors.New("ChainAddress.chain may only be set once")
	}

	c.chain = chain
	c.chainSet = true

	if c.addressSet {
		c.updateCasing()
	}

	return nil
}

func (c ChainAddress) String() string {
	return fmt.Sprintf("%d:%s", c.chain, c.address)
}

const (
	// WalletTypeEOA represents an externally owned account (regular wallet address)
	WalletTypeEOA WalletType = iota
	// WalletTypeGnosis represents a smart contract gnosis safe
	WalletTypeGnosis
)

// WalletRepository represents a repository for interacting with persisted wallets
type WalletRepository interface {
	GetByID(context.Context, DBID) (Wallet, error)
	GetByChainAddress(context.Context, ChainAddress) (Wallet, error)
	GetByUserID(context.Context, DBID) ([]Wallet, error)
	Insert(context.Context, ChainAddress, WalletType) (DBID, error)
}

func (l WalletList) Value() (driver.Value, error) {
	return pq.Array(l).Value()
}

// Scan implements the Scanner interface for the AddressList type
func (l *WalletList) Scan(value interface{}) error {
	return pq.Array(l).Scan(value)
}

// Scan implements the Scanner interface for the Wallet type
func (w *Wallet) Scan(value interface{}) error {
	if value == nil {
		*w = Wallet{}
		return nil
	}
	*w = Wallet{ID: DBID(string(value.([]uint8)))}
	return nil
}

// Value implements the database/sql driver Valuer interface for the Wallet type
func (w Wallet) Value() (driver.Value, error) {
	if w.ID == "" {
		return "", nil
	}
	return w.ID.String(), nil
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

	asString, ok := value.(string)
	if !ok {
		asUint8Array, ok := value.([]uint8)
		if !ok {
			return fmt.Errorf("Address must be a string or []uint8")
		}
		asString = string(asUint8Array)
	}

	*n = Address(asString)
	return nil
}

type ErrWalletAlreadyExists struct {
	WalletID     DBID
	ChainAddress ChainAddress
	OwnerID      DBID
}

// ErrWalletNotFound is an error type for when a wallet is not found
type ErrWalletNotFound struct {
	WalletID     DBID
	ChainAddress ChainAddress
}

func (e ErrWalletAlreadyExists) Error() string {
	return fmt.Sprintf("wallet already exists: wallet ID: %s | chain address: %s | chain: %d | owner ID: %s", e.WalletID, e.ChainAddress.Address(), e.ChainAddress.Chain(), e.OwnerID)
}

func (e ErrWalletNotFound) Error() string {
	return fmt.Sprintf("wallet not found: walletID: %s | chain address: %s | chain: %d ", e.WalletID, e.ChainAddress.Address(), e.ChainAddress.Chain())
}
