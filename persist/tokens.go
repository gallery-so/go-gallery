package persist

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
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

// TokenType represents the contract specification of the token
type TokenType string

// MediaType represents the type of media that a token
type MediaType string

// Chain represents which blockchain a token is on
type Chain string

// Token represents an individual Token token
type Token struct {
	Version      int64     `bson:"version"              json:"version"` // schema version for this model
	ID           DBID      `bson:"_id"                  json:"id" binding:"required"`
	CreationTime time.Time `bson:"created_at"        json:"created_at"`
	Deleted      bool      `bson:"deleted" json:"-"`
	LastUpdated  time.Time `bson:"last_updated" json:"last_updated"`

	CollectorsNote string `bson:"collectors_note" json:"collectors_note"`
	Media          Media  `bson:"media" json:"media"`

	TokenType TokenType `bson:"type" json:"type"`

	Chain Chain `bson:"chain" json:"chain"`

	Name        string `bson:"name" json:"name"`
	Description string `bson:"description" json:"description"`

	TokenURI        string                 `bson:"token_uri" json:"token_uri"`
	TokenID         string                 `bson:"token_id" json:"token_id"`
	Quantity        uint64                 `bson:"quantity" json:"quantity"`
	OwnerAddress    string                 `bson:"owner_address" json:"owner_address"`
	PreviousOwners  []string               `bson:"previous_owners" json:"previous_owners"`
	TokenMetadata   map[string]interface{} `bson:"token_metadata" json:"token_metadata"`
	ContractAddress string                 `bson:"contract_address" json:"contract_address"`

	ExternalURL string `bson:"external_url" json:"external_url"`

	LatestBlock uint64 `bson:"latest_block" json:"latest_block"`
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
	ID           DBID      `bson:"_id"                  json:"id" binding:"required"`
	CreationTime time.Time `bson:"created_at"        json:"created_at"`

	ContractAddress string `bson:"contract_address"     json:"contract_address"`

	Chain Chain `bson:"chain" json:"chain"`

	Name        string `bson:"name" json:"name"`
	Description string `bson:"description" json:"description"`

	TokenURI     string `bson:"token_uri" json:"token_uri"`
	TokenID      string `bson:"token_id" json:"token_id"`
	OwnerAddress string `bson:"owner_address" json:"owner_address"`

	Media         Media                  `bson:"media" json:"media"`
	TokenMetadata map[string]interface{} `bson:"token_metadata" json:"token_metadata"`
}

// TokenUpdateInfoInput represents a token update to update the token's user inputted info
type TokenUpdateInfoInput struct {
	CollectorsNote string `bson:"collectors_note" json:"collectors_note"`
}

// TokenUpdateMediaInput represents an update to a tokens image properties
type TokenUpdateMediaInput struct {
	Media *Media `bson:"media" json:"media"`
}

// TokenRepository represents a repository for interacting with persisted tokens
type TokenRepository interface {
	CreateBulk(context.Context, []*Token) ([]DBID, error)
	Create(context.Context, *Token) (DBID, error)
	GetByWallet(context.Context, string) ([]*Token, error)
	GetByUserID(context.Context, DBID) ([]*Token, error)
	GetByContract(context.Context, string) ([]*Token, error)
	GetByNFTIdentifiers(context.Context, string, string) (*Token, error)
	GetByID(context.Context, DBID) (*Token, error)
	BulkUpsert(context.Context, []*Token) error
	Upsert(context.Context, *Token) error
	UpdateByIDUnsafe(context.Context, DBID, interface{}) error
	UpdateByID(context.Context, DBID, DBID, interface{}) error
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
