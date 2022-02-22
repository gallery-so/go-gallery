package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/mikeydub/go-gallery/util"
)

const (
	// TokenTypeERC721 is the type of an ERC721 token
	TokenTypeERC721 TokenType = "ERC-721"
	// TokenTypeERC1155 is the type of an ERC1155 token
	TokenTypeERC1155 TokenType = "ERC-1155"
	// TokenTypeERC20 is the type of an ERC20 token
	TokenTypeERC20 TokenType = "ERC-20"
)

const (
	// MediaTypeVideo represents a video
	MediaTypeVideo MediaType = "video"
	// MediaTypeImage represents an image
	MediaTypeImage MediaType = "image"
	// MediaTypeGIF represents a gif
	MediaTypeGIF MediaType = "gif"
	// MediaTypeSVG represents an SVG
	MediaTypeSVG MediaType = "svg"
	// MediaTypeBase64JSON represents a base64 encoded JSON document
	MediaTypeBase64JSON MediaType = "base64json"
	// MediaTypeBase64SVG represents a base64 encoded SVG
	MediaTypeBase64SVG MediaType = "base64svg"
	// MediaTypeText represents plain text
	MediaTypeText MediaType = "text"
	// MediaTypeHTML represents html
	MediaTypeHTML MediaType = "html"
	// MediaTypeBase64Text represents a base64 encoded plain text
	MediaTypeBase64Text MediaType = "base64text"
	// MediaTypeAudio represents audio
	MediaTypeAudio MediaType = "audio"
	// MediaTypeJSON represents json metadata
	MediaTypeJSON MediaType = "json"
	// MediaTypeInvalid represents an invalid media type such as when a token's external metadata's API is broken or no longer exists
	MediaTypeInvalid MediaType = "invalid"
	// MediaTypeUnknown represents an unknown media type
	MediaTypeUnknown MediaType = "unknown"
)

const (
	// ChainETH represents the Ethereum blockchain
	ChainETH Chain = "ETH"
	// ChainArbitrum represents the Arbitrum blockchain
	ChainArbitrum Chain = "Arbitrum"
	// ChainPolygon represents the Polygon/Matic blockchain
	ChainPolygon Chain = "Polygon"
	// ChainOptimism represents the Optimism blockchain
	ChainOptimism Chain = "Optimism"
)

const (
	// URITypeIPFS represents an IPFS URI
	URITypeIPFS URIType = "ipfs"
	// URITypeArweave represents an Arweave URI
	URITypeArweave URIType = "arweave"
	// URITypeHTTP represents an HTTP URI
	URITypeHTTP URIType = "http"
	// URITypeIPFSAPI represents an IPFS API URI
	URITypeIPFSAPI URIType = "ipfs-api"
	// URITypeBase64JSON represents a base64 encoded JSON document
	URITypeBase64JSON URIType = "base64json"
	// URITypeJSON represents a JSON document
	URITypeJSON URIType = "json"
	// URITypeBase64SVG represents a base64 encoded SVG
	URITypeBase64SVG URIType = "base64svg"
	// URITypeSVG represents an SVG
	URITypeSVG URIType = "svg"
	// URITypeENS represents an ENS domain
	URITypeENS URIType = "ens"
	// URITypeUnknown represents an unknown URI type
	URITypeUnknown URIType = "unknown"
	// URITypeInvalid represents an invalid URI type
	URITypeInvalid URIType = "invalid"
	// URITypeNone represents no URI
	URITypeNone URIType = "none"
)

const (
	// CountTypeTotal represents the total count
	CountTypeTotal TokenCountType = "total"
	// CountTypeNoMetadata represents the count of tokens without metadata
	CountTypeNoMetadata TokenCountType = "no-metadata"
	// CountTypeERC721 represents the count of ERC721 tokens
	CountTypeERC721 TokenCountType = "erc721"
	// CountTypeERC1155 represents the count of ERC1155 tokens
	CountTypeERC1155 TokenCountType = "erc1155"
)

// InvalidTokenURI represents an invalid token URI
const InvalidTokenURI TokenURI = "INVALID"

// ZeroAddress is the all-zero Ethereum address
const ZeroAddress Address = "0x0000000000000000000000000000000000000000"

// Address represents an Ethereum address
type Address string

// BlockNumber represents an Ethereum block number
type BlockNumber uint64

// TokenType represents the contract specification of the token
type TokenType string

// MediaType represents the type of media that a token
type MediaType string

// URIType represents the type of a URI
type URIType string

// TokenCountType represents the query of a token count operation
type TokenCountType string

// Chain represents which blockchain a token is on
type Chain string

// TokenID represents the ID of an Ethereum token
type TokenID string

// TokenURI represents the URI for an Ethereum token
type TokenURI string

// TokenMetadata represents the JSON metadata for a token
type TokenMetadata map[string]interface{}

// HexString represents a hex number of any size
type HexString string

// AddressAtBlock is an address connected to a block number
type AddressAtBlock struct {
	Address Address     `bson:"address" json:"address"`
	Block   BlockNumber `bson:"block" json:"block"`
}

// Token represents an individual Token token
type Token struct {
	Version      NullInt64       `bson:"version"              json:"version"` // schema version for this model
	ID           DBID            `bson:"_id"                  json:"id" binding:"required"`
	CreationTime CreationTime    `bson:"created_at"        json:"created_at"`
	Deleted      NullBool        `bson:"deleted" json:"-"`
	LastUpdated  LastUpdatedTime `bson:"last_updated,update_time" json:"last_updated"`

	CollectorsNote NullString `bson:"collectors_note,omitempty" json:"collectors_note"`
	Media          Media      `bson:"media,omitempty" json:"media"`

	TokenType TokenType `bson:"token_type,omitempty" json:"token_type"`

	Chain Chain `bson:"chain,omitempty" json:"chain"`

	Name        NullString `bson:"name,omitempty" json:"name"`
	Description NullString `bson:"description,omitempty" json:"description"`

	TokenURI         TokenURI         `bson:"token_uri,omitempty" json:"token_uri"`
	TokenID          TokenID          `bson:"token_id" json:"token_id"`
	Quantity         HexString        `bson:"quantity,omitempty" json:"quantity"`
	OwnerAddress     Address          `bson:"owner_address,omitempty" json:"owner_address"`
	OwnershipHistory []AddressAtBlock `bson:"ownership_history,omitempty" json:"previous_owners"`
	TokenMetadata    TokenMetadata    `bson:"metadata,omitempty" json:"metadata"`
	ContractAddress  Address          `bson:"contract_address" json:"contract_address"`

	ExternalURL NullString `bson:"external_url,omitempty" json:"external_url"`

	BlockNumber BlockNumber `bson:"block_number,omitempty" json:"block_number"`
}

// Media represents a token's media content with processed images from metadata
type Media struct {
	ThumbnailURL NullString `json:"thumbnail_url,omitempty"`
	MediaURL     NullString `json:"media_url,omitempty"`
	MediaType    MediaType  `json:"media_type"`
}

// TokenInCollection represents a token within a collection
type TokenInCollection struct {
	ID           DBID         `bson:"_id"                  json:"id" binding:"required"`
	CreationTime CreationTime `bson:"created_at"        json:"created_at"`

	ContractAddress Address `bson:"contract_address"     json:"contract_address"`

	Chain Chain `bson:"chain" json:"chain"`

	Name        NullString `bson:"name" json:"name"`
	Description NullString `bson:"description" json:"description"`

	TokenType TokenType `bson:"token_type" json:"token_type"`

	TokenURI     TokenURI `bson:"token_uri" json:"token_uri"`
	TokenID      TokenID  `bson:"token_id" json:"token_id"`
	OwnerAddress Address  `bson:"owner_address" json:"owner_address"`

	Media         Media         `bson:"media" json:"media"`
	TokenMetadata TokenMetadata `bson:"metadata" json:"metadata"`
}

// TokenUpdateInfoInput represents a token update to update the token's user inputted info
type TokenUpdateInfoInput struct {
	LastUpdated LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	CollectorsNote NullString `bson:"collectors_note" json:"collectors_note"`
}

// TokenUpdateMediaInput represents an update to a tokens image properties
type TokenUpdateMediaInput struct {
	LastUpdated LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	Media    Media         `bson:"media" json:"media"`
	Metadata TokenMetadata `bson:"token_metadata" json:"token_metadata"`
	TokenURI TokenURI      `bson:"token_uri" json:"token_uri"`
}

// TokenRepository represents a repository for interacting with persisted tokens
type TokenRepository interface {
	CreateBulk(context.Context, []Token) ([]DBID, error)
	Create(context.Context, Token) (DBID, error)
	GetByWallet(context.Context, Address, int64, int64) ([]Token, error)
	GetByUserID(context.Context, DBID, int64, int64) ([]Token, error)
	GetByContract(context.Context, Address, int64, int64) ([]Token, error)
	GetByTokenIdentifiers(context.Context, TokenID, Address, int64, int64) ([]Token, error)
	GetByID(context.Context, DBID) (Token, error)
	BulkUpsert(context.Context, []Token) error
	Upsert(context.Context, Token) error
	UpdateByIDUnsafe(context.Context, DBID, interface{}) error
	UpdateByID(context.Context, DBID, DBID, interface{}) error
	UpdateByTokenIdentifiersUnsafe(context.Context, TokenID, Address, interface{}) error
	MostRecentBlock(context.Context) (BlockNumber, error)
	Count(context.Context, TokenCountType) (int64, error)
}

// ErrTokenNotFoundByIdentifiers is an error that is returned when a token is not found by its identifiers (token ID and contract address)
type ErrTokenNotFoundByIdentifiers struct {
	TokenID         TokenID
	ContractAddress Address
}

// ErrTokenNotFoundByID is an error that is returned when a token is not found by its ID
type ErrTokenNotFoundByID struct {
	ID DBID
}

// SniffMediaType will attempt to detect the media type for a given array of bytes
func SniffMediaType(buf []byte) MediaType {

	contentType := http.DetectContentType(buf)
	return MediaFromContentType(contentType)
}

// MediaFromContentType will attempt to convert a content type to a media type
func MediaFromContentType(contentType string) MediaType {
	whereCharset := strings.IndexByte(contentType, ';')
	if whereCharset != -1 {
		contentType = contentType[:whereCharset]
	}
	spl := strings.Split(contentType, "/")

	switch spl[0] {
	case "image":
		switch spl[1] {
		case "svg":
			return MediaTypeSVG
		case "gif":
			return MediaTypeGIF
		default:
			return MediaTypeImage
		}
	case "video":
		return MediaTypeVideo
	case "audio":
		return MediaTypeAudio
	case "text":
		switch spl[1] {
		case "html":
			return MediaTypeHTML
		default:
			return MediaTypeText
		}
	default:
		return MediaTypeUnknown
	}
}

func (e ErrTokenNotFoundByID) Error() string {
	return fmt.Sprintf("token not found by ID: %s", e.ID)
}

func (e ErrTokenNotFoundByIdentifiers) Error() string {
	return fmt.Sprintf("token not found with contract address %v and token ID %v", e.ContractAddress, e.TokenID)
}

// Value implements the driver.Valuer interface for the Chain type
func (c Chain) Value() (driver.Value, error) {
	return string(c), nil
}

// Scan implements the sql.Scanner interface for the Chain type
func (c *Chain) Scan(src interface{}) error {
	if src == nil {
		*c = Chain("")
		return nil
	}
	*c = Chain(src.(string))
	return nil
}

// URL turns a token's URI into a URL
func (uri TokenURI) URL() (*url.URL, error) {
	return url.Parse(uri.String())
}

func (uri TokenURI) String() string {
	asString := string(uri)
	if strings.HasPrefix(asString, "http") || strings.HasPrefix(asString, "ipfs") || strings.HasPrefix(asString, "ar") {
		url, err := url.QueryUnescape(string(uri))
		if err == nil && url != string(uri) {
			return url
		}
	}
	return asString
}

// Value implements the driver.Valuer interface for token URIs
func (uri TokenURI) Value() (driver.Value, error) {
	result := string(uri)
	if strings.Contains(result, "://") {
		result = url.QueryEscape(result)
	}
	return strings.ToValidUTF8(result, ""), nil
}

// ReplaceID replaces the token's ID with the given ID
func (uri TokenURI) ReplaceID(id TokenID) TokenURI {
	return TokenURI(strings.TrimSpace(strings.ReplaceAll(uri.String(), "{id}", id.ToUint256String())))
}

// Scan implements the sql.Scanner interface for token URIs
func (uri *TokenURI) Scan(src interface{}) error {
	if src == nil {
		*uri = TokenURI("")
		return nil
	}
	*uri = TokenURI(src.(string))
	return nil
}

// Type returns the type of the token URI
func (uri TokenURI) Type() URIType {
	asString := uri.String()
	asString = strings.TrimSpace(asString)
	switch {
	case strings.HasPrefix(asString, "ipfs"), strings.HasPrefix(asString, "Qm"):
		return URITypeIPFS
	case strings.HasPrefix(asString, "ar://"), strings.HasPrefix(asString, "arweave://"):
		return URITypeArweave
	case strings.HasPrefix(asString, "data:application/json;base64,"):
		return URITypeBase64JSON
	case strings.HasPrefix(asString, "data:image/svg+xml;base64,"), strings.HasPrefix(asString, "data:image/svg xml;base64,"):
		return URITypeBase64SVG
	case strings.Contains(asString, "ipfs.io/api"):
		return URITypeIPFSAPI
	case strings.HasPrefix(asString, "http"), strings.HasPrefix(asString, "https"):
		return URITypeHTTP
	case strings.HasPrefix(asString, "{"), strings.HasPrefix(asString, "["), strings.HasPrefix(asString, "data:application/json"), strings.HasPrefix(asString, "data:text/plain,{"):
		return URITypeJSON
	case strings.HasPrefix(asString, "<svg"), strings.HasPrefix(asString, "data:image/svg+xml"):
		return URITypeSVG
	case strings.HasSuffix(asString, ".ens"):
		return URITypeENS
	case asString == InvalidTokenURI.String():
		return URITypeInvalid
	case asString == "":
		return URITypeNone
	default:
		return URITypeUnknown
	}
}

func (id TokenID) String() string {
	return strings.ToLower(util.RemoveLeftPaddedZeros(string(id)))
}

// Value implements the driver.Valuer interface for token IDs
func (id TokenID) Value() (driver.Value, error) {
	return id.String(), nil
}

// Scan implements the sql.Scanner interface for token IDs
func (id *TokenID) Scan(src interface{}) error {
	if src == nil {
		*id = TokenID("")
		return nil
	}
	*id = TokenID(src.(string))
	return nil
}

// BigInt returns the token ID as a big.Int
func (id TokenID) BigInt() *big.Int {
	normalized := util.RemoveLeftPaddedZeros(string(id))
	if normalized == "" {
		return big.NewInt(0)
	}
	i, ok := new(big.Int).SetString(normalized, 16)

	if !ok {
		panic(fmt.Sprintf("failed to parse token ID %s as base 16", normalized))
	}

	return i
}

// ToUint256String returns the uint256 hex string representation of the token id
func (id TokenID) ToUint256String() string {
	return fmt.Sprintf("%064s", id.String())
}

// Base10String returns the token ID as a base 10 string
func (id TokenID) Base10String() string {
	return id.BigInt().String()
}

// Base10Int returns the token ID as a base 10 integer
func (id TokenID) Base10Int() int64 {
	return id.BigInt().Int64()
}

func (hex HexString) String() string {
	return strings.TrimPrefix(strings.ToLower(string(hex)), "0x")
}

// Value implements the driver.Valuer interface for hex strings
func (hex HexString) Value() (driver.Value, error) {
	return hex.String(), nil
}

// Scan implements the sql.Scanner interface for hex strings
func (hex *HexString) Scan(src interface{}) error {
	if src == nil {
		*hex = HexString("")
		return nil
	}
	*hex = HexString(src.(string))
	return nil
}

// BigInt returns the hex string as a big.Int
func (hex HexString) BigInt() *big.Int {
	it, ok := big.NewInt(0).SetString(hex.String(), 16)
	if !ok {
		it, _ = big.NewInt(0).SetString(hex.String(), 10)
	}
	return it
}

// Value implements the driver.Valuer interface for media
func (m Media) Value() (driver.Value, error) {
	return json.Marshal(m)
}

// Scan implements the sql.Scanner interface for media
func (m *Media) Scan(src interface{}) error {
	if src == nil {
		*m = Media{}
		return nil
	}
	return json.Unmarshal(src.([]uint8), &m)
}

func (a Address) String() string {
	return normalizeAddress(strings.ToLower(string(a)))
}

// Address returns the ethereum address byte array
func (a Address) Address() common.Address {
	return common.HexToAddress(a.String())
}

// Value implements the database/sql/driver Valuer interface for the address type
func (a Address) Value() (driver.Value, error) {
	return a.String(), nil
}

// MarshallJSON implements the json.Marshaller interface for the address type
func (a Address) MarshallJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

// UnmarshalJSON implements the json.Unmarshaller interface for the address type
func (a *Address) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*a = Address(normalizeAddress(strings.ToLower(s)))
	return nil
}

// Scan implements the database/sql Scanner interface
func (a *Address) Scan(i interface{}) error {
	if i == nil {
		*a = Address("")
		return nil
	}
	if it, ok := i.(string); ok {
		*a = Address(it)
		return nil
	}
	*a = Address(i.([]uint8))
	return nil
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

// Value implements the database/sql/driver Valuer interface for the block number type
func (b BlockNumber) Value() (driver.Value, error) {
	return b.BigInt().Int64(), nil
}

// Scan implements the database/sql Scanner interface for the block number type
func (b *BlockNumber) Scan(src interface{}) error {
	if src == nil {
		*b = BlockNumber(0)
		return nil
	}
	*b = BlockNumber(src.(int64))
	return nil
}

// Scan implements the database/sql Scanner interface for the TokenMetadata type
func (m *TokenMetadata) Scan(src interface{}) error {
	if src == nil {
		*m = TokenMetadata{}
		return nil
	}
	return json.Unmarshal(src.([]uint8), m)
}

// Value implements the database/sql/driver Valuer interface for the TokenMetadata type
func (m TokenMetadata) Value() (driver.Value, error) {
	return json.Marshal(m)
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

// Value implements the database/sql/driver Valuer interface for the MediaType type
func (m MediaType) Value() (driver.Value, error) {
	return string(m), nil
}

// Scan implements the database/sql Scanner interface for the MediaType type
func (m *MediaType) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	*m = MediaType(src.(string))
	return nil
}

func normalizeAddress(address string) string {
	withoutPrefix := strings.TrimPrefix(address, "0x")
	if len(withoutPrefix) < 40 {
		return ""
	}
	return "0x" + withoutPrefix[len(withoutPrefix)-40:]
}
