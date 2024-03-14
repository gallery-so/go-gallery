package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"blockwatch.cc/tzgo/tezos"
	"github.com/lib/pq"
)

// Wallet represents an address on any chain
type Wallet struct {
	ID           DBID      `json:"id"`
	Version      NullInt64 `json:"version"`
	CreationTime time.Time `json:"created_at"`
	Deleted      NullBool  `json:"-"`
	LastUpdated  time.Time `json:"last_updated"`

	Address    Address    `json:"address"`
	Chain      Chain      `json:"chain"`
	L1Chain    L1Chain    `json:"l1_chain"`
	WalletType WalletType `json:"wallet_type"`
}

// WalletType is the type of wallet used to sign a message
type WalletType int

type WalletList []Wallet

// Address represents the value of an address
type Address string

// PubKey represents the public key of a wallet
type PubKey string

type ChainAddress struct {
	addressSet bool
	chainSet   bool
	address    Address
	chain      Chain
}

type L1ChainAddress struct {
	addressSet bool
	chainSet   bool
	address    Address
	l1Chain    L1Chain
}

type ChainPubKey struct {
	pubKeySet bool
	chainSet  bool
	pubKey    PubKey
	chain     Chain
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

func (c ChainAddress) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"address": c.address.String(), "chain": int(c.chain)})
}

func (c *ChainAddress) UnmarshalJSON(data []byte) error {
	var v map[string]any
	err := json.Unmarshal(data, &v)
	if err != nil {
		return err
	}

	if v["address"] != nil {
		c.address = Address(v["address"].(string))
		c.addressSet = true
	}

	if v["chain"] != nil {
		if chain, ok := v["chain"].(int); ok {
			c.chain = Chain(chain)
			c.chainSet = true
		} else if chain, ok := v["chain"].(int32); ok {
			c.chain = Chain(chain)
			c.chainSet = true
		}
	}

	return nil
}

func (c ChainAddress) ToL1ChainAddress() L1ChainAddress {
	return NewL1ChainAddress(c.Address(), c.Chain())
}

func (c *L1ChainAddress) IsGalleryUserOrAddress() {}

func NewL1ChainAddress(address Address, chain Chain) L1ChainAddress {
	ca := L1ChainAddress{
		addressSet: true,
		chainSet:   true,
		address:    address,
		l1Chain:    chain.L1Chain(),
	}

	ca.updateCasing()
	return ca
}

func (c *L1ChainAddress) Address() Address {
	return c.address
}

func (c *L1ChainAddress) L1Chain() L1Chain {
	return c.l1Chain
}

func (c *L1ChainAddress) updateCasing() {
	switch c.l1Chain {
	// TODO: Add an IsCaseSensitive to the Chain type?
	case L1Chain(ChainETH):
		c.address = Address(strings.ToLower(c.address.String()))
	}
}

func (c L1ChainAddress) String() string {
	return fmt.Sprintf("%d:%s", c.l1Chain, c.address)
}

func (c L1ChainAddress) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"address": c.address.String(), "chain": int(c.l1Chain)})
}

func (c *L1ChainAddress) UnmarshalJSON(data []byte) error {
	var v map[string]any
	err := json.Unmarshal(data, &v)
	if err != nil {
		return err
	}

	if v["address"] != nil {
		c.address = Address(v["address"].(string))
		c.addressSet = true
	}

	if v["chain"] != nil {
		if chain, ok := v["chain"].(int); ok {
			c.l1Chain = L1Chain(chain)
			c.chainSet = true
		} else if chain, ok := v["chain"].(int32); ok {
			c.l1Chain = L1Chain(chain)
			c.chainSet = true
		} else if chain, ok := v["chain"].(Chain); ok {
			c.l1Chain = L1Chain(chain)
			c.chainSet = true
		}
	}

	return nil
}

func NewChainPubKey(pubKey PubKey, chain Chain) ChainPubKey {
	ca := ChainPubKey{
		pubKeySet: true,
		chainSet:  true,
		pubKey:    pubKey,
		chain:     chain,
	}

	ca.updateCasing()
	return ca
}

func (c *ChainPubKey) PubKey() PubKey {
	return c.pubKey
}

func (c *ChainPubKey) Chain() Chain {
	return c.chain
}

func (c *ChainPubKey) updateCasing() {
	switch c.chain {
	// TODO: Add an IsCaseSensitive to the Chain type?
	case ChainETH:
		c.pubKey = PubKey(strings.ToLower(c.pubKey.String()))
	}
}

// GQLSetPubKeyFromResolver will be called automatically from the required gqlgen resolver and should
// never be called manually. To set a ChainPubKey's fields, use NewChainPubKey.
func (c *ChainPubKey) GQLSetPubKeyFromResolver(pubKey PubKey) error {
	if c.pubKeySet {
		return errors.New("ChainAddress.address may only be set once")
	}

	c.pubKey = pubKey
	c.pubKeySet = true

	if c.chainSet {
		c.updateCasing()
	}

	return nil
}

// GQLSetChainFromResolver will be called automatically from the required gqlgen resolver and should
// never be called manually. To set a ChainPubKey's fields, use NewChainPubKey.
func (c *ChainPubKey) GQLSetChainFromResolver(chain Chain) error {
	if c.chainSet {
		return errors.New("ChainAddress.chain may only be set once")
	}

	c.chain = chain
	c.chainSet = true

	if c.pubKeySet {
		c.updateCasing()
	}

	return nil
}

func (c ChainPubKey) String() string {
	return fmt.Sprintf("%d:%s", c.chain, c.pubKey)
}

// ToChainAddress converts a chain pub key to a chain address
func (c ChainPubKey) ToChainAddress() ChainAddress {
	switch c.chain {
	case ChainTezos:
		key, err := tezos.ParseKey(c.pubKey.String())
		if err != nil {
			panic(err)
		}
		return NewChainAddress(Address(key.Address().String()), c.chain)
	default:
		return NewChainAddress(Address(c.pubKey), c.chain)
	}
}

func (c ChainPubKey) ToL1ChainAddress() L1ChainAddress {
	return c.ToChainAddress().ToL1ChainAddress()
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
	n, ok := v.(string)
	if !ok {
		return fmt.Errorf("wrong type for WalletType: %T", v)
	}
	switch n {
	case "EOA":
		*wa = WalletTypeEOA
	case "GnosisSafe":
		*wa = WalletTypeGnosis
	default:
		return fmt.Errorf("unknown WalletType: %s", n)
	}
	return nil
}

// MarshalGQL implements the graphql.Marshaler interface
func (wa WalletType) MarshalGQL(w io.Writer) {
	switch wa {
	case WalletTypeEOA:
		w.Write([]byte(`"EOA"`))
	case WalletTypeGnosis:
		w.Write([]byte(`"GnosisSafe"`))
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

func (p PubKey) String() string {
	return string(p)
}

type ErrWalletAlreadyExists struct {
	WalletID       DBID
	L1ChainAddress L1ChainAddress
	OwnerID        DBID
}

func (e ErrWalletAlreadyExists) Error() string {
	return fmt.Sprintf("wallet already exists: wallet ID: %s | chain address: %s | chain: %d | owner ID: %s", e.WalletID, e.L1ChainAddress.Address(), e.L1ChainAddress.L1Chain(), e.OwnerID)
}

var errWalletNotFound ErrWalletNotFound

type ErrWalletNotFound struct{}

func (e ErrWalletNotFound) Unwrap() error { return notFoundError }
func (e ErrWalletNotFound) Error() string { return "wallet not found" }

type ErrWalletNotFoundByID struct{ ID DBID }

func (e ErrWalletNotFoundByID) Unwrap() error { return errWalletNotFound }
func (e ErrWalletNotFoundByID) Error() string { return "wallet not found by id: " + e.ID.String() }

type ErrWalletNotFoundByAddress struct{ Address L1ChainAddress }

func (e ErrWalletNotFoundByAddress) Unwrap() error { return errWalletNotFound }
func (e ErrWalletNotFoundByAddress) Error() string {
	return fmt.Sprintf("wallet not found by chain=%d; address = %s", e.Address.L1Chain(), e.Address.Address())
}
