package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// TokenGallery represents an individual Token
type TokenGallery struct {
	Version      NullInt32       `json:"version"` // schema version for this model
	ID           DBID            `json:"id" binding:"required"`
	CreationTime CreationTime    `json:"created_at"`
	Deleted      NullBool        `json:"-"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`

	CollectorsNote NullString `json:"collectors_note"`
	Media          Media      `json:"media"`

	TokenType TokenType `json:"token_type"`

	Chain Chain `json:"chain"`

	Name        NullString `json:"name"`
	Description NullString `json:"description"`

	TokenURI         TokenURI  `json:"token_uri"`
	TokenID          TokenID   `json:"token_id"`
	Quantity         HexString `json:"quantity"`
	OwnerUserID      DBID
	OwnedByWallets   []Wallet         `json:"owned_by_wallets"`
	OwnershipHistory []AddressAtBlock `json:"previous_owners"`
	TokenMetadata    TokenMetadata    `json:"metadata"`
	Contract         DBID             `json:"contract"`

	ExternalURL NullString `json:"external_url"`

	BlockNumber BlockNumber `json:"block_number"`
}

// AddressAtBlock represents an address at a specific block
type AddressAtBlock struct {
	Address Address     `json:"address"`
	Block   BlockNumber `json:"block"`
}

// TokenIdentifiers represents a unique identifier for a token
type TokenIdentifiers struct {
	TokenID         TokenID `json:"token_id"`
	ContractAddress Address `json:"contract_address"`
	Chain           Chain   `json:"chain"`
}

// TokenInCollection represents a token within a collection
type TokenInCollection struct {
	ID           DBID         `json:"id" binding:"required"`
	CreationTime CreationTime `json:"created_at"`

	ContractAddress DBID `json:"contract_address"`

	Chain Chain `json:"chain"`

	Name        NullString `json:"name"`
	Description NullString `json:"description"`

	TokenType TokenType `json:"token_type"`

	TokenURI     TokenURI `json:"token_uri"`
	TokenID      TokenID  `json:"token_id"`
	OwnerAddress DBID     `json:"owner_address"`

	Media         Media         `json:"media"`
	TokenMetadata TokenMetadata `json:"metadata"`
}

// TokenUpdateInfoInput represents a token update to update the token's user inputted info
type TokenUpdateInfoInput struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	CollectorsNote NullString `json:"collectors_note"`
}

// TokenUpdateMediaInput represents an update to a tokens image properties
type TokenUpdateMediaInput struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	Media       Media         `json:"media"`
	Metadata    TokenMetadata `json:"token_metadata"`
	TokenURI    TokenURI      `json:"token_uri"`
	Name        NullString    `json:"name"`
	Description NullString    `json:"description"`
}

// TokenGalleryRepository represents a repository for interacting with persisted tokens
type TokenGalleryRepository interface {
	CreateBulk(context.Context, []TokenGallery) ([]DBID, error)
	Create(context.Context, TokenGallery) (DBID, error)
	GetByUserID(context.Context, DBID, int64, int64) ([]TokenGallery, error)
	GetByContract(context.Context, Address, Chain, int64, int64) ([]TokenGallery, error)
	GetByTokenIdentifiers(context.Context, TokenID, Address, Chain, int64, int64) ([]TokenGallery, error)
	GetByTokenID(context.Context, TokenID, int64, int64) ([]TokenGallery, error)
	GetByID(context.Context, DBID) (TokenGallery, error)
	BulkUpsert(context.Context, []TokenGallery) error
	Upsert(context.Context, TokenGallery) error
	UpdateByIDUnsafe(context.Context, DBID, interface{}) error
	UpdateByID(context.Context, DBID, DBID, interface{}) error
	UpdateByTokenIdentifiersUnsafe(context.Context, TokenID, Address, Chain, interface{}) error
	MostRecentBlock(context.Context) (BlockNumber, error)
	Count(context.Context, TokenCountType) (int64, error)
	DeleteByID(context.Context, DBID) error
}

type ErrTokensGalleryNotFoundByContract struct {
	ContractAddress Address
	Chain           Chain
}

type ErrTokenGalleryNotFoundByIdentifiers struct {
	TokenID         TokenID
	ContractAddress Address
	Chain           Chain
}

// NewTokenIdentifiers creates a new token identifiers
func NewTokenIdentifiers(pContractAddress Address, pTokenID TokenID, pChain Chain) TokenIdentifiers {
	return TokenIdentifiers{
		TokenID:         pTokenID,
		ContractAddress: pContractAddress,
		Chain:           pChain,
	}
}

func (t TokenIdentifiers) String() string {
	return fmt.Sprintf("%s+%s+%d", t.Chain.NormalizeAddress(t.ContractAddress), t.TokenID, t.Chain)
}

// Value implements the driver.Valuer interface
func (t TokenIdentifiers) Value() (driver.Value, error) {
	return t.String(), nil
}

// Scan implements the database/sql Scanner interface for the TokenIdentifiers type
func (t *TokenIdentifiers) Scan(i interface{}) error {
	if i == nil {
		*t = TokenIdentifiers{}
		return nil
	}
	res := strings.Split(i.(string), "+")
	if len(res) != 2 {
		return fmt.Errorf("invalid token identifiers: %v - %T", i, i)
	}
	chain, err := strconv.Atoi(res[2])
	if err != nil {
		return err
	}
	*t = TokenIdentifiers{
		TokenID:         TokenID(res[1]),
		ContractAddress: Address(res[0]),
		Chain:           Chain(chain),
	}
	return nil
}

// Scan implements the database/sql Scanner interface for the AddressAtBlock type
func (a *AddressAtBlock) Scan(src interface{}) error {
	if src == nil {
		*a = AddressAtBlock{}
		return nil
	}
	return json.Unmarshal(src.([]uint8), a)
}

// Value implements the database/sql/driver Valuer interface for the AddressAtBlock type
func (a AddressAtBlock) Value() (driver.Value, error) {
	return json.Marshal(a)
}

func (e ErrTokensGalleryNotFoundByContract) Error() string {
	return fmt.Sprintf("tokens not found by contract: %s", e.ContractAddress)
}

func (e ErrTokenGalleryNotFoundByIdentifiers) Error() string {
	return fmt.Sprintf("token not found with contract address %v and token ID %v", e.ContractAddress, e.TokenID)
}
