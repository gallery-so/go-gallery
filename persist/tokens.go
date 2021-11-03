package persist

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"

	"github.com/mikeydub/go-gallery/util"
)

const (
	// TokenTypeERC721 is the type of an ERC721 token
	TokenTypeERC721 TokenType = "ERC-721"
	// TokenTypeERC1155 is the type of an ERC1155 token
	TokenTypeERC1155 TokenType = "ERC-1155"
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
	// MediaTypeBase64Text represents a base64 encoded plain text
	MediaTypeBase64Text MediaType = "base64text"
	// MediaTypeAudio represents audio
	MediaTypeAudio MediaType = "audio"
	// MediaTypeJSON represents audio
	MediaTypeJSON MediaType = "json"
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
	// URITypeHTTP represents an HTTP URI
	URITypeHTTP URIType = "http"
	// URITypeIPFSAPI represents an IPFS API URI
	URITypeIPFSAPI URIType = "ipfs-api"
	// URITypeBase64JSON represents a base64 encoded JSON document
	URITypeBase64JSON URIType = "base64json"
	// URITypeBase64SVG represents a base64 encoded SVG
	URITypeBase64SVG URIType = "base64svg"
	// URITypeUnknown represents an unknown URI type
	URITypeUnknown URIType = "unknown"
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
	Version      int64           `bson:"version"              json:"version"` // schema version for this model
	ID           DBID            `bson:"_id"                  json:"id" binding:"required"`
	CreationTime CreationTime    `bson:"created_at"        json:"created_at"`
	Deleted      bool            `bson:"deleted" json:"-"`
	LastUpdated  LastUpdatedTime `bson:"last_updated,update_time" json:"last_updated"`

	CollectorsNote string `bson:"collectors_note,omitempty" json:"collectors_note"`
	Media          Media  `bson:"media,omitempty" json:"media"`

	TokenType TokenType `bson:"token_type,omitempty" json:"token_type"`

	Chain Chain `bson:"chain,omitempty" json:"chain"`

	Name        string `bson:"name,omitempty" json:"name"`
	Description string `bson:"description,omitempty" json:"description"`

	TokenURI        TokenURI         `bson:"token_uri,omitempty" json:"token_uri"`
	TokenID         TokenID          `bson:"token_id" json:"token_id"`
	Quantity        HexString        `bson:"quantity,omitempty" json:"quantity"`
	OwnerAddress    Address          `bson:"owner_address,omitempty" json:"owner_address"`
	PreviousOwners  []AddressAtBlock `bson:"previous_owners,omitempty" json:"previous_owners"`
	TokenMetadata   TokenMetadata    `bson:"token_metadata,omitempty" json:"token_metadata"`
	ContractAddress Address          `bson:"contract_address" json:"contract_address"`

	ExternalURL string `bson:"external_url,omitempty" json:"external_url"`

	BlockNumber BlockNumber `bson:"block_number,omitempty" json:"block_number"`
}

// Media represents a token's media content with processed images from metadata
type Media struct {
	ThumbnailURL string    `bson:"thumbnail_url" json:"thumbnail_url"`
	PreviewURL   string    `bson:"preview_url" json:"preview_url"`
	MediaURL     string    `bson:"media_url" json:"media_url"`
	MediaType    MediaType `bson:"media_type" json:"media_type"`
}

// TokenInCollection represents a token within a collection
type TokenInCollection struct {
	ID           DBID         `bson:"_id"                  json:"id" binding:"required"`
	CreationTime CreationTime `bson:"created_at"        json:"created_at"`

	ContractAddress Address `bson:"contract_address"     json:"contract_address"`

	Chain Chain `bson:"chain" json:"chain"`

	Name        string `bson:"name" json:"name"`
	Description string `bson:"description" json:"description"`

	TokenType TokenType `bson:"token_type" json:"token_type"`

	TokenURI     TokenURI `bson:"token_uri" json:"token_uri"`
	TokenID      TokenID  `bson:"token_id" json:"token_id"`
	OwnerAddress Address  `bson:"owner_address" json:"owner_address"`

	Media         Media         `bson:"media" json:"media"`
	TokenMetadata TokenMetadata `bson:"token_metadata" json:"token_metadata"`
}

// TokenUpdateInfoInput represents a token update to update the token's user inputted info
type TokenUpdateInfoInput struct {
	CollectorsNote string `bson:"collectors_note" json:"collectors_note"`
}

// TokenUpdateMediaInput represents an update to a tokens image properties
type TokenUpdateMediaInput struct {
	Media    Media         `bson:"media" json:"media"`
	Metadata TokenMetadata `bson:"token_metadata" json:"token_metadata"`
}

// TokenRepository represents a repository for interacting with persisted tokens
type TokenRepository interface {
	CreateBulk(context.Context, []*Token) ([]DBID, error)
	Create(context.Context, *Token) (DBID, error)
	GetByWallet(context.Context, Address, int64, int64) ([]*Token, error)
	GetByUserID(context.Context, DBID, int64, int64) ([]*Token, error)
	GetByContract(context.Context, Address, int64, int64) ([]*Token, error)
	GetByTokenIdentifiers(context.Context, TokenID, Address, int64, int64) ([]*Token, error)
	GetByID(context.Context, DBID) (*Token, error)
	BulkUpsert(context.Context, []*Token) error
	Upsert(context.Context, *Token) error
	UpdateByIDUnsafe(context.Context, DBID, interface{}) error
	UpdateByID(context.Context, DBID, DBID, interface{}) error
	MostRecentBlock(context.Context) (BlockNumber, error)
	Count(context.Context, TokenCountType) (int64, error)
}

// ErrTokenNotFoundByIdentifiers is an error that is returned when a token is not found by its identifiers (token ID and contract address)
type ErrTokenNotFoundByIdentifiers struct {
	TokenID, ContractAddress string
}

// ErrTokenNotFoundByID is an error that is returned when a token is not found by its ID
type ErrTokenNotFoundByID struct {
	ID DBID
}

// SniffMediaType will attempt to detect the media type for a given array of bytes
func SniffMediaType(buf []byte) MediaType {
	contentType := http.DetectContentType(buf[:512])
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
		return MediaTypeText
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

// URL turns a token's URI into a URL
func (uri TokenURI) URL() (*url.URL, error) {
	return url.Parse(uri.String())
}

func (uri TokenURI) String() string {
	return string(uri)
}

// Type returns the type of the token URI
func (uri TokenURI) Type() URIType {
	asString := uri.String()
	switch {
	case strings.Contains(asString, "ipfs://"):
		return URITypeIPFS
	case strings.Contains(asString, "data:application/json;base64,"):
		return URITypeBase64JSON
	case strings.Contains(asString, "data:image/svg+xml;base64,"):
		return URITypeBase64SVG
	case strings.Contains(asString, "ipfs.io/api"):
		return URITypeIPFSAPI
	case strings.Contains(asString, "http://"), strings.Contains(asString, "https://"):
		return URITypeHTTP
	default:
		return URITypeUnknown
	}
}

func (id TokenID) String() string {
	return strings.ToLower(util.RemoveLeftPaddedZeros(string(id)))
}

// BigInt returns the token ID as a big.Int
func (id TokenID) BigInt() *big.Int {
	i, _ := new(big.Int).SetString(id.String(), 16)
	return i
}

func (hex HexString) String() string {
	return strings.TrimPrefix(strings.ToLower(string(hex)), "0x")
}
