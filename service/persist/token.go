package persist

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgtype"
	"io"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"

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
	// MediaTypeText represents plain text
	MediaTypeText MediaType = "text"
	// MediaTypeHTML represents html
	MediaTypeHTML MediaType = "html"
	// MediaTypeAudio represents audio
	MediaTypeAudio MediaType = "audio"
	// MediaTypeJSON represents json metadata
	MediaTypeJSON MediaType = "json"
	// MediaTypeAnimation represents an animation (.glb)
	MediaTypeAnimation MediaType = "animation"
	// MediaTypePDF represents a pdf
	MediaTypePDF MediaType = "pdf"
	// MediaTypeInvalid represents an invalid media type such as when a token's external metadata's API is broken or no longer exists
	MediaTypeInvalid MediaType = "invalid"
	// MediaTypeUnknown represents an unknown media type
	MediaTypeUnknown MediaType = "unknown"
	// MediaTypeSyncing represents a syncing media
	MediaTypeSyncing MediaType = "syncing"
	// MediaTypeFallback represents a fallback media
	MediaTypeFallback MediaType = "fallback"
)

var mediaTypePriorities = []MediaType{MediaTypeHTML, MediaTypeAudio, MediaTypeAnimation, MediaTypeVideo, MediaTypeGIF, MediaTypeSVG, MediaTypeImage, MediaTypeJSON, MediaTypeText, MediaTypeSyncing, MediaTypeUnknown, MediaTypeInvalid}

func (m MediaType) ToContentType() string {
	switch m {
	case MediaTypeVideo:
		return "video/mp4"
	case MediaTypeImage:
		return "image/jpeg"
	case MediaTypeGIF:
		return "image/gif"
	case MediaTypeSVG:
		return "image/svg+xml"
	case MediaTypeText:
		return "text/plain"
	case MediaTypeHTML:
		return "text/html"
	case MediaTypeAudio:
		return "audio/mpeg"
	case MediaTypeJSON:
		return "application/json"
	case MediaTypeAnimation:
		return "model/gltf-binary"
	case MediaTypePDF:
		return "application/pdf"
	default:
		return ""
	}
}

const (
	// ChainETH represents the Ethereum blockchain
	ChainETH Chain = iota
	// ChainArbitrum represents the Arbitrum blockchain
	ChainArbitrum
	// ChainPolygon represents the Polygon/Matic blockchain
	ChainPolygon
	// ChainOptimism represents the Optimism blockchain
	ChainOptimism
	// ChainTezos represents the Tezos blockchain
	ChainTezos
	// ChainPOAP represents a POAP
	ChainPOAP
	// ChainZora represents the Zora blockchain
	ChainZora
	// ChainBase represents the base chain
	ChainBase
	ChainBaseSepolia = Chain(84532)

	// MaxChainValue is the highest valid chain value, and should always be updated to
	// point to the most recently added chain type.
	MaxChainValue = ChainBase
)

func MustTokenID(s string) HexTokenID {
	return HexTokenID(MustHexString(s))
}

func MustHexString(s string) HexString {
	base := 10

	if strings.HasPrefix(s, "0x") {
		s = strings.TrimPrefix(s, "0x")
		base = 16
	}

	v, ok := new(big.Int).SetString(s, base)
	if !ok {
		panic(fmt.Sprintf("failed to convert '%s' to a number", s))
	}

	return HexString(v.Text(16))
}

var L1Chains = map[Chain]L1Chain{
	ChainPOAP:        L1Chain(ChainETH),
	ChainOptimism:    L1Chain(ChainETH),
	ChainPolygon:     L1Chain(ChainETH),
	ChainArbitrum:    L1Chain(ChainETH),
	ChainZora:        L1Chain(ChainETH),
	ChainBase:        L1Chain(ChainETH),
	ChainETH:         L1Chain(ChainETH),
	ChainTezos:       L1Chain(ChainTezos),
	ChainBaseSepolia: L1Chain(ChainETH),
}

var L1ChainGroups = map[L1Chain][]Chain{
	L1Chain(ChainETH):   EvmChains,
	L1Chain(ChainTezos): {ChainTezos},
}

var AllChains = []Chain{ChainETH, ChainArbitrum, ChainPolygon, ChainOptimism, ChainTezos, ChainPOAP, ChainZora, ChainBase}
var EvmChains = util.MapKeys(evmChains)
var evmChains map[Chain]bool = map[Chain]bool{
	ChainETH:      true,
	ChainOptimism: true,
	ChainPolygon:  true,
	ChainArbitrum: true,
	ChainPOAP:     true,
	ChainZora:     true,
	ChainBase:     true,
}

const (
	// URITypeIPFS represents an IPFS URI
	URITypeIPFS URIType = "ipfs"
	// URITypeArweave represents an Arweave URI
	URITypeArweave URIType = "arweave"
	// URITypeHTTP represents an HTTP URI
	URITypeHTTP URIType = "http"
	// URITypeIPFSAPI represents an IPFS API URI
	URITypeIPFSAPI URIType = "ipfs-api"
	// URITypeIPFSGateway represents an IPFS Gateway URI
	URITypeIPFSGateway URIType = "ipfs-gateway"
	// URITypeArweaveGateway represents an Arweave Gateway URI
	URITypeArweaveGateway URIType = "arweave-gateway"
	// URITypeBase64JSON represents a base64 encoded JSON document
	URITypeBase64JSON URIType = "base64json"
	// URITypeBase64HTML represents a base64 encoded HTML document
	URITypeBase64HTML URIType = "base64html"
	// URITypeJSON represents a JSON document
	URITypeJSON URIType = "json"
	// URITypeBase64SVG represents a base64 encoded SVG
	URITypeBase64SVG URIType = "base64svg"
	//URITypeBase64BMP represents a base64 encoded BMP
	URITypeBase64BMP URIType = "base64bmp"
	// URITypeBase64PNG represents a base64 encoded PNG
	URITypeBase64PNG URIType = "base64png"
	// URITypeBase64JPEG represents a base64 encoded JPEG
	URITypeBase64JPEG URIType = "base64jpeg"
	// URITypeBase64GIF represents a base64 encoded GIF
	URITypeBase64GIF URIType = "base64gif"
	// URITypeBase64WAV represents a base64 encoded WAV
	URITypeBase64WAV URIType = "base64wav"
	// URITypeBase64MP3 represents a base64 encoded MP3
	URITypeBase64MP3 URIType = "base64mp3"
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

func (u URIType) IsRaw() bool {
	switch u {
	case URITypeBase64JSON, URITypeBase64HTML, URITypeBase64SVG, URITypeBase64BMP, URITypeBase64PNG, URITypeBase64JPEG, URITypeBase64GIF, URITypeBase64WAV, URITypeBase64MP3, URITypeJSON, URITypeSVG, URITypeENS:
		return true
	default:
		return false
	}
}

func (u URIType) ToMediaType() MediaType {
	switch u {
	case URITypeBase64JSON, URITypeJSON:
		return MediaTypeJSON
	case URITypeBase64SVG, URITypeSVG:
		return MediaTypeSVG
	case URITypeBase64BMP:
		return MediaTypeImage
	case URITypeBase64PNG:
		return MediaTypeImage
	case URITypeBase64HTML:
		return MediaTypeHTML
	case URITypeBase64JPEG:
		return MediaTypeImage
	case URITypeBase64GIF:
		return MediaTypeGIF
	case URITypeBase64MP3:
		return MediaTypeAudio
	case URITypeBase64WAV:
		return MediaTypeAudio
	default:
		return MediaTypeUnknown
	}
}

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

const (
	TokenOwnershipTypeHolder  TokenOwnershipType = "holder"
	TokenOwnershipTypeCreator TokenOwnershipType = "creator"
)

// InvalidTokenURI represents an invalid token URI
const InvalidTokenURI TokenURI = "INVALID"

// ZeroAddress is the all-zero Ethereum address
const ZeroAddress EthereumAddress = "0x0000000000000000000000000000000000000000"

// EthereumAddress represents an Ethereum address
type EthereumAddress string

// EthereumAddressList is a slice of Addresses, used to implement scanner/valuer interfaces
type EthereumAddressList []EthereumAddress

func (l EthereumAddressList) Value() (driver.Value, error) {
	return pq.Array(l).Value()
}

// Scan implements the Scanner interface for the AddressList type
func (l *EthereumAddressList) Scan(value interface{}) error {
	return pq.Array(l).Scan(value)
}

// BlockNumber represents an Ethereum block number
type BlockNumber uint64

// BlockRange represents an inclusive block range
type BlockRange [2]BlockNumber

// TokenType represents the contract specification of the token
type TokenType string

// MediaType represents the type of media that a token
type MediaType string

// URIType represents the type of a URI
type URIType string

// TokenCountType represents the query of a token count operation
type TokenCountType string

// Chain represents which blockchain a token is on
type Chain int

type L1Chain Chain

type DecimalTokenID string
type DecimalTokenIDList []DecimalTokenID

func (l DecimalTokenIDList) Value() (driver.Value, error) {
	return pq.Array(l).Value()
}

func (l *DecimalTokenIDList) Scan(value interface{}) error {
	return pq.Array(l).Scan(value)
}

// HexTokenID represents the ID of a token in hexadecimal
type HexTokenID string

type HexTokenIDList []HexTokenID

func (l HexTokenIDList) Value() (driver.Value, error) {
	return pq.Array(l).Value()
}

func (l *HexTokenIDList) Scan(value interface{}) error {
	return pq.Array(l).Scan(value)
}

// TokenURI represents the URI for an Ethereum token
type TokenURI string

// TokenMetadata represents the JSON metadata for a token
type TokenMetadata map[string]interface{}

// HexString represents a hex number of any size
type HexString string

// EthereumAddressAtBlock is an address connected to a block number
type EthereumAddressAtBlock struct {
	Address EthereumAddress `json:"address"`
	Block   BlockNumber     `json:"block"`
}

// EthereumTokenIdentifiers represents a unique identifier for a token on the Ethereum Blockchain
type EthereumTokenIdentifiers string

type TokenOwnershipType string

func (t TokenOwnershipType) String() string {
	return string(t)
}

// Token represents an individual Token token
type Token struct {
	Version      NullInt32 `json:"version"` // schema version for this model
	ID           DBID      `json:"id" binding:"required"`
	CreationTime time.Time `json:"created_at"`
	Deleted      NullBool  `json:"-"`
	LastUpdated  time.Time `json:"last_updated"`

	TokenType TokenType `json:"token_type"`

	Chain Chain `json:"chain"`

	Name        NullString `json:"name"`
	Description NullString `json:"description"`

	TokenURI         TokenURI                 `json:"token_uri"`
	TokenID          HexTokenID               `json:"token_id"`
	Quantity         HexString                `json:"quantity"`
	OwnerAddress     EthereumAddress          `json:"owner_address"`
	OwnershipHistory []EthereumAddressAtBlock `json:"previous_owners"`
	TokenMetadata    TokenMetadata            `json:"metadata"`
	ContractAddress  EthereumAddress          `json:"contract_address"`

	ExternalURL NullString `json:"external_url"`

	BlockNumber BlockNumber `json:"block_number"`
	IsSpam      *bool       `json:"is_spam"`
}

type Dimensions struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (d Dimensions) Valid() bool {
	return d.Width > 0 && d.Height > 0
}

type FallbackMedia struct {
	ImageURL   NullString `json:"image_url,omitempty"`
	Dimensions Dimensions `json:"dimensions"`
}

// ContractCollectionNFT represents a contract within a collection nft
type ContractCollectionNFT struct {
	ContractName  NullString `json:"name"`
	ContractImage NullString `json:"image_url"`
}

type TokenUpdateOwnerInput struct {
	OwnerAddress EthereumAddress `json:"owner_address"`
	BlockNumber  BlockNumber     `json:"block_number"`
}

type TokenUpdateBalanceInput struct {
	Quantity    HexString   `json:"quantity"`
	BlockNumber BlockNumber `json:"block_number"`
}

var errTokenNotFound ErrTokenNotFound

type ErrTokenNotFound struct{}

func (e ErrTokenNotFound) Unwrap() error { return notFoundError }
func (e ErrTokenNotFound) Error() string { return "token not found" }

// ErrTokenNotFoundByTokenIdentifiers is an error that is returned when a token is not found by its identifiers (token ID and contract address)
type ErrTokenNotFoundByTokenIdentifiers struct {
	Token TokenIdentifiers
}

func (e ErrTokenNotFoundByTokenIdentifiers) Unwrap() error { return errTokenNotFound }
func (e ErrTokenNotFoundByTokenIdentifiers) Error() string {
	return fmt.Sprintf("token not found by identifiers: %s", e.Token.String())
}

// ErrTokenNotFoundByID is an error that is returned when a token is not found by its ID
type ErrTokenNotFoundByID struct {
	ID DBID
}

func (e ErrTokenNotFoundByID) Unwrap() error { return errTokenNotFound }
func (e ErrTokenNotFoundByID) Error() string {
	return fmt.Sprintf("token not found by ID: %s", e.ID)
}

type ErrTokenNotFoundByUserTokenIdentifers struct {
	UserID DBID
	Token  TokenIdentifiers
}

func (e ErrTokenNotFoundByUserTokenIdentifers) Unwrap() error { return errTokenNotFound }
func (e ErrTokenNotFoundByUserTokenIdentifers) Error() string {
	return fmt.Sprintf("token not found by user ID: %s and identifiers: %s", e.UserID, e.Token.String())
}

type ErrTokensNotFoundByContract struct {
	ContractAddress EthereumAddress
}

func (e ErrTokensNotFoundByContract) Error() string {
	return fmt.Sprintf("tokens not found by contract: %s", e.ContractAddress)
}

func (c Chain) String() string {
	switch c {
	case ChainETH:
		return "ethereum"
	case ChainArbitrum:
		return "arbitrum"
	case ChainPolygon:
		return "polygon"
	case ChainOptimism:
		return "optimism"
	case ChainTezos:
		return "tezos"
	case ChainPOAP:
		return "poap"
	case ChainZora:
		return "zora"
	case ChainBase:
		return "base"
	default:
		return strconv.Itoa(int(c))
	}
}

// NormalizeAddress normalizes an address for the given chain
func (c Chain) NormalizeAddress(addr Address) string {
	if evmChains[c] {
		return strings.ToLower(addr.String())
	}
	return addr.String()
}

// BaseKeywords are the keywords that are default for discovering media for a given chain
func (c Chain) BaseKeywords() (image []string, anim []string) {
	defaultImageKeyWords := []string{"image_url", "image", "imageOriginal"}
	defaultAnimKeyWords := []string{"animation_url", "animation", "video", "mediaOriginal"}
	// We map Simplehash's metadata to a standard metadata format that use the default keys above
	// so we shouldn't need to add more chain-specific keywords. However, we continue to use them for backwards compatibility
	// in case we revert back to older providers.
	switch c {
	case ChainTezos:
		return append(defaultImageKeyWords, "displayUri", "image", "thumbnailUri", "artifactUri", "uri"), append(defaultAnimKeyWords, "artifactUri", "displayUri", "uri", "image")
	default:
		return defaultImageKeyWords, defaultAnimKeyWords
	}
}

// Value implements the driver.Valuer interface for the Chain type
func (c Chain) Value() (driver.Value, error) {
	return int64(c), nil
}

// Scan implements the sql.Scanner interface for the Chain type
func (c *Chain) Scan(src interface{}) error {
	if src == nil {
		*c = Chain(0)
		return nil
	}
	*c = Chain(src.(int64))
	return nil
}

// UnmarshalJSON will unmarshall the JSON data into the Chain type
func (c *Chain) UnmarshalJSON(data []byte) error {
	var s int
	var asString string
	if err := json.Unmarshal(data, &s); err != nil {
		err = json.Unmarshal(data, &asString)
		if err != nil {
			return err
		}
		switch strings.ToLower(asString) {
		case "ethereum":
			*c = ChainETH
		case "tezos":
			*c = ChainTezos
		case "arbitrum":
			*c = ChainArbitrum
		case "polygon":
			*c = ChainPolygon
		case "optimism":
			*c = ChainOptimism
		case "poap":
			*c = ChainPOAP
		case "zora":
			*c = ChainZora
		case "base":
			*c = ChainBase
		}
		return nil
	}
	*c = Chain(s)
	return nil
}

// UnmarshalGQL implements the graphql.Unmarshaler interface
func (c *Chain) UnmarshalGQL(v interface{}) error {
	n, ok := v.(string)
	if !ok {
		return fmt.Errorf("Chain must be an string")
	}

	switch strings.ToLower(n) {
	case "ethereum":
		*c = ChainETH
	case "arbitrum":
		*c = ChainArbitrum
	case "polygon":
		*c = ChainPolygon
	case "optimism":
		*c = ChainOptimism
	case "tezos":
		*c = ChainTezos
	case "poap":
		*c = ChainPOAP
	case "zora":
		*c = ChainZora
	case "base":
		*c = ChainBase
	}
	return nil
}

// MarshalGQL implements the graphql.Marshaler interface
func (c Chain) MarshalGQL(w io.Writer) {
	switch c {
	case ChainETH:
		w.Write([]byte(`"Ethereum"`))
	case ChainTezos:
		w.Write([]byte(`"Tezos"`))
	case ChainPOAP:
		w.Write([]byte(`"POAP"`))
	case ChainArbitrum:
		w.Write([]byte(`"Arbitrum"`))
	case ChainPolygon:
		w.Write([]byte(`"Polygon"`))
	case ChainOptimism:
		w.Write([]byte(`"Optimism"`))
	case ChainZora:
		w.Write([]byte(`"Zora"`))
	case ChainBase:
		w.Write([]byte(`"Base"`))
	}
}

func (c Chain) L1Chain() L1Chain {
	lc, ok := L1Chains[c]
	if !ok {
		panic("l1 chain not found")
	}
	return lc
}

func (c Chain) L1ChainGroup() []Chain {
	cg, ok := L1ChainGroups[c.L1Chain()]
	if !ok {
		panic("chain group not found")
	}
	return cg
}

func (uri TokenURI) String() string { return string(uri) }

// Value implements the driver.Valuer interface for token URIs
func (uri TokenURI) Value() (driver.Value, error) {
	result := string(uri)
	if strings.Contains(result, "://") {
		result = url.QueryEscape(result)
	}
	clean := strings.Map(cleanString, result)
	return strings.ToValidUTF8(strings.ReplaceAll(clean, "\\u0000", ""), ""), nil
}

// ReplaceID replaces the token's ID with the given ID
func (uri TokenURI) ReplaceID(id HexTokenID) TokenURI {
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
	case strings.HasPrefix(asString, "data:text/html;base64,"), strings.HasPrefix(asString, "data:text/html;charset=utf-8;base64,"), strings.HasPrefix(asString, "data:text/html") && strings.Contains(asString, ";base64,"):
		return URITypeBase64HTML
	case strings.HasPrefix(asString, "data:application/json;base64,"), strings.HasPrefix(asString, "data:application/json;charset=utf-8;base64,"), strings.HasPrefix(asString, "data:application/json") && strings.Contains(asString, ";base64,"):
		return URITypeBase64JSON
	case strings.HasPrefix(asString, "data:image/svg+xml;base64,"), strings.HasPrefix(asString, "data:image/svg xml;base64,"), strings.HasPrefix(asString, "data:image/svg+xml") && strings.Contains(asString, ";base64,"), strings.HasPrefix(asString, "data:image/svg xml") && strings.Contains(asString, ";base64,"):
		return URITypeBase64SVG
	case strings.HasPrefix(asString, "data:image/bmp;base64,"), strings.HasPrefix(asString, "data:image/bmp;charset=utf-8;base64,"), strings.HasPrefix(asString, "data:image/bmp") && strings.Contains(asString, ";base64,"):
		return URITypeBase64BMP
	case strings.HasPrefix(asString, "data:image/png;base64,"), strings.HasPrefix(asString, "data:image/png;charset=utf-8;base64,"), strings.HasPrefix(asString, "data:image/png") && strings.Contains(asString, ";base64,"):
		return URITypeBase64PNG
	case strings.HasPrefix(asString, "data:image/jpeg;base64,"), strings.HasPrefix(asString, "data:image/jpeg;charset=utf-8;base64,"), strings.HasPrefix(asString, "data:image/jpeg") && strings.Contains(asString, ";base64,"):
		return URITypeBase64JPEG
	case strings.HasPrefix(asString, "data:image/gif;base64,"), strings.HasPrefix(asString, "data:image/gif;charset=utf-8;base64,"), strings.HasPrefix(asString, "data:image/gif") && strings.Contains(asString, ";base64,"):
		return URITypeBase64GIF
	case strings.HasPrefix(asString, "data:audio/wav;base64,"), strings.HasPrefix(asString, "data:audio/wav;charset=utf-8;base64,"), strings.HasPrefix(asString, "data:audio/wav") && strings.Contains(asString, ";base64,"):
		return URITypeBase64WAV
	case strings.HasPrefix(asString, "data:audio/mpeg;base64,"), strings.HasPrefix(asString, "data:audio/mpeg;charset=utf-8;base64,"), strings.HasPrefix(asString, "data:audio/mpeg") && strings.Contains(asString, ";base64,"):
		return URITypeBase64MP3
	case strings.Contains(asString, "ipfs.io/api"):
		return URITypeIPFSAPI
	case strings.Contains(asString, "/ipfs/"):
		return URITypeIPFSGateway
	case strings.HasPrefix(asString, "https://arweave.net/"):
		return URITypeArweaveGateway
	case strings.HasPrefix(asString, "http"), strings.HasPrefix(asString, "https"):
		return URITypeHTTP
	case strings.HasPrefix(asString, "{"), strings.HasPrefix(asString, "["), strings.HasPrefix(asString, "data:application/json"), strings.HasPrefix(asString, "data:text/plain,{"):
		return URITypeJSON
	case strings.HasPrefix(asString, "<svg"), strings.HasPrefix(asString, "data:image/svg+xml"), strings.HasPrefix(asString, "data:image/svg xml"):
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

// IsRenderable returns whether a frontend could render the given URI directly
func (uri TokenURI) IsRenderable() bool {
	return uri.IsHTTP() // || uri.IsIPFS() || uri.IsArweave()
}

// IsHTTP returns whether a frontend could render the given URI directly
func (uri TokenURI) IsHTTP() bool {
	asString := uri.String()
	asString = strings.TrimSpace(asString)
	return strings.HasPrefix(asString, "http")
}

func (id DecimalTokenID) String() string {
	return util.RemoveLeftPaddedZeros(string(id))
}

// Value implements the driver.Valuer interface for token IDs
func (id DecimalTokenID) Value() (driver.Value, error) {
	if id == "" {
		return nil, nil
	}

	num := pgtype.Numeric{}
	err := num.Set(id)
	if err != nil {
		return nil, err
	}

	return num, nil
}

func (id *DecimalTokenID) DecodeBinary(ci *pgtype.ConnInfo, src []byte) error {
	// Using the db/sql Scanner interface to get Postgres "numeric" types is complicated because
	// Postgres will output values as strings in scientific notation. Using pgx's DecodeBinary
	// interface avoids this issue and lets us decode directly to a pgtype.Numeric without an
	// intermediate string or the need to parse scientific notation.
	var numeric pgtype.Numeric
	err := numeric.DecodeBinary(ci, src)
	if err != nil {
		return err
	}

	if numeric.Status == pgtype.Null {
		*id = ""
		return nil
	}

	// Assign pgtype.Numeric to big.Rat, because pgtype.Numeric has built-in support for this conversion
	var rat big.Rat
	if err := numeric.AssignTo(&rat); err != nil {
		return fmt.Errorf("cannot assign pgtype.Numeric to big.Rat: %w", err)
	}

	// Use FloatString with 0 decimal places to convert the big.Rat to a whole number
	*id = DecimalTokenID(rat.FloatString(0))

	return nil
}

func expandNumericString(s string) (string, error) {
	// Split the string on 'e'. If 'e' is not present, parts will contain the original string as its only element.
	parts := strings.Split(s, "e")
	if len(parts) == 1 {
		// No 'e' in the string, return it as is.
		return s, nil
	} else if len(parts) != 2 {
		// More than one 'e' found, or some unexpected format.
		return "", fmt.Errorf("string does not contain a valid format: %s", s)
	}

	base := parts[0]
	expPart := parts[1]

	// Convert the exponent part to an integer.
	exponent, err := strconv.Atoi(expPart)
	if err != nil {
		return "", fmt.Errorf("invalid exponent: %s", expPart)
	}

	// If the exponent is negative or zero, the original string format might not make sense for simple expansion.
	if exponent < 0 {
		return "", fmt.Errorf("negative exponent not handled: %d", exponent)
	}

	// Append the required number of zeros to the base.
	for i := 0; i < exponent; i++ {
		base += "0"
	}

	return base, nil
}

// Scan implements the sql.Scanner interface for token IDs
func (id *DecimalTokenID) Scan(src interface{}) error {
	if src == nil {
		*id = ""
		return nil
	}

	if str, ok := src.(string); ok {
		expanded, err := expandNumericString(str)
		if err != nil {
			return err
		}
		*id = DecimalTokenID(expanded)
		return nil
	}

	return fmt.Errorf("cannot convert %T to DecimalTokenID", src)
}

func (id DecimalTokenID) Numeric() pgtype.Numeric {
	num := pgtype.Numeric{}
	err := num.Set(id)
	if err != nil {
		panic(fmt.Sprintf("failed to convert %s to pgtype.Numeric: %s", id, err))
	}
	return num
}

// BigInt returns the token ID as a big.Int
func (id DecimalTokenID) BigInt() *big.Int {
	normalized := id.String()
	if normalized == "" {
		return big.NewInt(0)
	}
	i, ok := new(big.Int).SetString(normalized, 10)

	if !ok {
		panic(fmt.Sprintf("failed to parse token ID %s as base 10", normalized))
	}

	return i
}

// ToUint256String returns the uint256 hex string representation of the token id
// TODO: Unsure if we need this for decimal IDs
//func (id DecimalTokenID) ToUint256String() string {
//	return fmt.Sprintf("%064s", id.String())
//}

// Base10String returns the token ID as a base 10 string
func (id DecimalTokenID) ToHexTokenID() HexTokenID {
	return HexTokenID(id.BigInt().Text(16))
}

// ToInt returns the token ID as a base 10 integer
// TODO: We should look at places where we use this, since an int64 can't hold all possible TokenIDs
//func (id DecimalTokenID) ToInt() int64 {
//	return id.BigInt().Int64()
//}

// UnmarshalGQL implements the graphql.Unmarshaler interface
func (id *DecimalTokenID) UnmarshalGQL(v any) error {
	val, ok := v.(string)
	if !ok {
		return fmt.Errorf("failed to convert %s to an integer", v)
	}

	if i, ok := new(big.Int).SetString(val, 10); ok {
		*id = DecimalTokenID(i.Text(10))
		return nil
	}

	return fmt.Errorf("failed to convert %s to an integer", val)
}

// MarshalGQL implements the graphql.Marshaler interface
func (id DecimalTokenID) MarshalGQL(w io.Writer) {
	w.Write([]byte(fmt.Sprintf(`"%s"`, id.String())))
}

func (id HexTokenID) String() string {
	return strings.ToLower(util.RemoveLeftPaddedZeros(string(id)))
}

// Value implements the driver.Valuer interface for token IDs
func (id HexTokenID) Value() (driver.Value, error) {
	return id.String(), nil
}

// Scan implements the sql.Scanner interface for token IDs
func (id *HexTokenID) Scan(src interface{}) error {
	if src == nil {
		*id = HexTokenID("")
		return nil
	}
	*id = HexTokenID(src.(string))
	return nil
}

// BigInt returns the token ID as a big.Int
func (id HexTokenID) BigInt() *big.Int {
	normalized := id.String()
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
func (id HexTokenID) ToUint256String() string {
	return fmt.Sprintf("%064s", id.String())
}

// Base10String returns the token ID as a base 10 string
func (id HexTokenID) Base10String() string {
	return id.BigInt().String()
}

func (id HexTokenID) ToDecimalTokenID() DecimalTokenID {
	return DecimalTokenID(id.BigInt().Text(10))
}

// ToInt returns the token ID as a base 10 integer
func (id HexTokenID) ToInt() int64 {
	return id.BigInt().Int64()
}

// UnmarshalGQL implements the graphql.Unmarshaler interface
func (id *HexTokenID) UnmarshalGQL(v any) error {
	if val, ok := v.(string); ok {
		// Assume its in hexadecimal
		if strings.HasPrefix(val, "0x") {
			*id = HexTokenID(HexTokenID(val).String())
			return nil
		}
		// Assume its in decimal
		asInt, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("failed to convert %s; prepend with '0x' if val is in hex", val)
		}
		*id = HexTokenID(fmt.Sprintf("%x", asInt))
	}
	return nil
}

// MarshalGQL implements the graphql.Marshaler interface
func (id HexTokenID) MarshalGQL(w io.Writer) {
	p := "0x" + id.String()
	w.Write([]byte(fmt.Sprintf(`"%s"`, p)))
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
		it, ok = big.NewInt(0).SetString(hex.String(), 10)
		if !ok {
			return big.NewInt(0)
		}
	}
	return it
}

// Add adds the given hex string to the current hex string
func (hex HexString) Add(new HexString) HexString {
	asInt := hex.BigInt()
	return HexString(asInt.Add(asInt, new.BigInt()).Text(16))
}

// IsServable returns true if the token's Media has enough information to serve it's assets.
func (m FallbackMedia) IsServable() bool {
	return m.ImageURL != ""
}

// Value implements the driver.Valuer interface for media
func (m FallbackMedia) Value() (driver.Value, error) {
	return json.Marshal(m)
}

// Scan implements the sql.Scanner interface for media
func (m *FallbackMedia) Scan(src interface{}) error {
	if src == nil {
		*m = FallbackMedia{}
		return nil
	}
	return json.Unmarshal(src.([]byte), &m)
}

func (a EthereumAddress) String() string {
	return normalizeAddress(strings.ToLower(string(a)))
}

// Address returns the ethereum address byte array
func (a EthereumAddress) Address() common.Address {
	return common.HexToAddress(a.String())
}

// Value implements the database/sql/driver Valuer interface for the address type
func (a EthereumAddress) Value() (driver.Value, error) {
	return a.String(), nil
}

// MarshallJSON implements the json.Marshaller interface for the address type
func (a EthereumAddress) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

// UnmarshalJSON implements the json.Unmarshaller interface for the address type
func (a *EthereumAddress) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*a = EthereumAddress(normalizeAddress(strings.ToLower(s)))
	return nil
}

// Scan implements the database/sql Scanner interface
func (a *EthereumAddress) Scan(i interface{}) error {
	if i == nil {
		*a = EthereumAddress("")
		return nil
	}
	if it, ok := i.(string); ok {
		*a = EthereumAddress(it)
		return nil
	}
	*a = EthereumAddress(i.([]uint8))
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
	return m.MarshalJSON()
}

// MarshalJSON implements the json.Marshaller interface for the TokenMetadata type
func (m TokenMetadata) MarshalJSON() ([]byte, error) {
	asMap := map[string]interface{}(m)
	val, err := json.Marshal(asMap)
	if err != nil {
		return nil, err
	}
	cleaned := strings.ToValidUTF8(string(val), "")
	// Replace literal '\\u0000' with empty string (marshal to JSON escapes each backslash)
	cleaned = strings.ReplaceAll(cleaned, "\\\\u0000", "")
	// Replace unicode NULL char (u+0000) i.e. '\u0000' with empty string
	cleaned = strings.ReplaceAll(cleaned, "\\u0000", "")
	return []byte(cleaned), nil
}

// Scan implements the database/sql Scanner interface for the AddressAtBlock type
func (a *EthereumAddressAtBlock) Scan(src interface{}) error {
	if src == nil {
		*a = EthereumAddressAtBlock{}
		return nil
	}
	return json.Unmarshal(src.([]uint8), a)
}

// Value implements the database/sql/driver Valuer interface for the AddressAtBlock type
func (a EthereumAddressAtBlock) Value() (driver.Value, error) {
	return json.Marshal(a)
}

// IsValid returns true if the media type is not unknown, syncing, or invalid
func (m MediaType) IsValid() bool {
	return m != MediaTypeUnknown && m != MediaTypeInvalid && m != MediaTypeSyncing && m != ""
}

// IsImageLike returns true if the media type is a type that is expected to be like an image and not live render
func (m MediaType) IsImageLike() bool {
	return m == MediaTypeImage || m == MediaTypeGIF || m == MediaTypeSVG
}

// IsAnimationLike returns true if the media type is a type that is expected to be like an animation and live render
func (m MediaType) IsAnimationLike() bool {
	return m == MediaTypeVideo || m == MediaTypeHTML || m == MediaTypeAudio || m == MediaTypeAnimation
}

// IsMorePriorityThan returns true if the media type is more important than the other media type
func (m MediaType) IsMorePriorityThan(other MediaType) bool {
	for _, t := range mediaTypePriorities {
		if t == m {
			return true
		}
		if t == other {
			return false
		}
	}
	return true
}

// Value implements the database/sql/driver Valuer interface for the MediaType type
func (m MediaType) Value() (driver.Value, error) {
	return m.String(), nil
}

func (m MediaType) String() string {
	return string(m)
}

// Scan implements the database/sql Scanner interface for the MediaType type
func (m *MediaType) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	*m = MediaType(src.(string))
	return nil
}

func (t TokenType) String() string {
	return string(t)
}

// Value implements the database/sql/driver Valuer interface for the TokenType type
func (t TokenType) Value() (driver.Value, error) {
	return t.String(), nil
}

// Scan implements the database/sql Scanner interface for the TokenType type
func (t *TokenType) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	*t = TokenType(src.(string))
	return nil
}

// NewEthereumTokenIdentifiers creates a new token identifiers
func NewEthereumTokenIdentifiers(pContractAddress EthereumAddress, pTokenID HexTokenID) EthereumTokenIdentifiers {
	return EthereumTokenIdentifiers(fmt.Sprintf("%s+%s", pContractAddress, pTokenID))
}

func (t EthereumTokenIdentifiers) String() string {
	return string(t)
}

// GetParts returns the parts of the token identifiers
func (t EthereumTokenIdentifiers) GetParts() (EthereumAddress, HexTokenID, error) {
	parts := strings.Split(t.String(), "+")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid token identifiers: %s", t)
	}
	return EthereumAddress(EthereumAddress(parts[0]).String()), HexTokenID(HexTokenID(parts[1]).String()), nil
}

// Value implements the driver.Valuer interface
func (t EthereumTokenIdentifiers) Value() (driver.Value, error) {
	return t.String(), nil
}

// Scan implements the database/sql Scanner interface for the TokenIdentifiers type
func (t *EthereumTokenIdentifiers) Scan(i interface{}) error {
	if i == nil {
		*t = ""
		return nil
	}
	res := strings.Split(i.(string), "+")
	if len(res) != 2 {
		return fmt.Errorf("invalid token identifiers: %v - %T", i, i)
	}
	*t = EthereumTokenIdentifiers(fmt.Sprintf("%s+%s", res[0], res[1]))
	return nil
}

func normalizeAddress(address string) string {
	withoutPrefix := strings.TrimPrefix(address, "0x")
	if len(withoutPrefix) < 40 {
		return ""
	}
	return "0x" + withoutPrefix[len(withoutPrefix)-40:]
}

func WalletsToEthereumAddresses(pWallets []Wallet) []EthereumAddress {
	result := make([]EthereumAddress, len(pWallets))
	for i, wallet := range pWallets {
		result[i] = EthereumAddress(wallet.Address)
	}
	return result
}

// UnmarshalGQL implements the graphql.Unmarshaler interface
func (t *TokenOwnershipType) UnmarshalGQL(v interface{}) error {
	n, ok := v.(string)
	if !ok {
		return fmt.Errorf("TokenOwnershipType must be a string")
	}

	switch strings.ToLower(n) {
	case "holder":
		*t = TokenOwnershipTypeHolder
	case "creator":
		*t = TokenOwnershipTypeCreator
	}
	return nil
}

// MarshalGQL implements the graphql.Marshaler interface
func (t TokenOwnershipType) MarshalGQL(w io.Writer) {
	switch t {
	case TokenOwnershipTypeHolder:
		w.Write([]byte(`"holder"`))
	case TokenOwnershipTypeCreator:
		w.Write([]byte(`"creator"`))
	}
}
