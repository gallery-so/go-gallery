package persist

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	tokenColName = "tokens"
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

// TTB represents time til blockchain so that data isn't old in DB
var TTB = time.Minute * 10

// TokenType represents the contract specification of the token
type TokenType string

// MediaType represents the type of media that a token
type MediaType string

// Token represents an individual Token token
type Token struct {
	Version      int64              `bson:"version"              json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"                  json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at"        json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`
	LastUpdated  primitive.DateTime `bson:"last_updated" json:"last_updated"`

	CollectorsNote string    `bson:"collectors_note" json:"collectors_note"`
	ThumbnailURL   string    `bson:"thumbnail_url" json:"thumbnail_url"`
	PreviewURL     string    `bson:"preview_url" json:"preview_url"`
	MediaURL       string    `bson:"media_url" json:"media_url"`
	MediaType      MediaType `bson:"media_type" json:"media_type"`

	TokenType TokenType `bson:"type" json:"type"`

	TokenURI        string                 `bson:"token_uri" json:"token_uri"`
	TokenID         string                 `bson:"token_id" json:"token_id"`
	Amount          uint64                 `bson:"amount" json:"amount"`
	OwnerAddress    string                 `bson:"owner_address" json:"owner_address"`
	PreviousOwners  []string               `bson:"previous_owners" json:"previous_owners"`
	TokenMetadata   map[string]interface{} `bson:"token_metadata" json:"token_metadata"`
	ContractAddress string                 `bson:"contract_address" json:"contract_address"`

	LatestBlock uint64 `bson:"latest_block" json:"latest_block"`
}

// CollectionToken represents a token within a collection
type CollectionToken struct {
	ID           DBID               `bson:"_id"                  json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at"        json:"created_at"`

	ContractAddress string `bson:"contract_address"     json:"contract_address"`

	ThumbnailURL  string                 `bson:"thumbnail_url" json:"thumbnail_url"`
	PreviewURL    string                 `bson:"preview_url" json:"preview_url"`
	TokenMetadata map[string]interface{} `bson:"token_metadata" json:"token_metadata"`
}

// TokenUpdateInfoInput represents a token update to update the token's user inputted info
type TokenUpdateInfoInput struct {
	CollectorsNote string `bson:"collectors_note" json:"collectors_note"`
}

// TokenUpdateImageURLsInput represents an update to a tokens image properties
type TokenUpdateImageURLsInput struct {
	ThumbnailURL string `bson:"thumbnail_url" json:"thumbnail_url"`
	PreviewURL   string `bson:"preview_url" json:"preview_url"`
	MediaURL     string `bson:"media_url" json:"media_url"`
}

// TokenCreateBulk is a helper function to create multiple nfts in one call and returns
// the ids of each nft created
func TokenCreateBulk(pCtx context.Context, pERC721s []*Token,
	pRuntime *runtime.Runtime) ([]DBID, error) {

	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

	nfts := make([]interface{}, len(pERC721s))

	for i, v := range pERC721s {
		nfts[i] = v
	}

	ids, err := mp.insertMany(pCtx, nfts)

	if err != nil {
		return nil, err
	}
	return ids, nil
}

// TokenCreate inserts a token into the database
func TokenCreate(pCtx context.Context, pERC721 *Token,
	pRuntime *runtime.Runtime) (DBID, error) {

	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

	return mp.insert(pCtx, pERC721)
}

// TokenGetByWallet gets tokens for a given wallet address
func TokenGetByWallet(pCtx context.Context, pAddress string, pPageNumber int, pMaxCount int,
	pRuntime *runtime.Runtime) ([]*Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	opts.SetSort(bson.M{"last_updated": -1})

	if pPageNumber > 0 && pMaxCount > 0 {
		opts.SetSkip(int64((pPageNumber - 1) * pMaxCount))
		opts.SetLimit(int64(pMaxCount))
	}

	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

	result := []*Token{}

	err := mp.find(pCtx, bson.M{"owner_address": strings.ToLower(pAddress)}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// TokenGetByUserID gets ERC721 tokens for a given userID
func TokenGetByUserID(pCtx context.Context, pUserID DBID, pPageNumber int, pMaxCount int,
	pRuntime *runtime.Runtime) ([]*Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	opts.SetSort(bson.M{"last_updated": -1})

	if pPageNumber > 0 && pMaxCount > 0 {
		opts.SetSkip(int64((pPageNumber - 1) * pMaxCount))
		opts.SetLimit(int64(pMaxCount))
	}

	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

	result := []*Token{}

	err := mp.find(pCtx, bson.M{"owner_user_id": pUserID}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// TokenGetByContract gets ERC721 tokens for a given contract
func TokenGetByContract(pCtx context.Context, pAddress string, pPageNumber int, pMaxCount int,
	pRuntime *runtime.Runtime) ([]*Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	opts.SetSort(bson.M{"last_updated": -1})

	if pPageNumber > 0 && pMaxCount > 0 {
		opts.SetSkip(int64((pPageNumber - 1) * pMaxCount))
		opts.SetLimit(int64(pMaxCount))
	}

	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

	result := []*Token{}

	err := mp.find(pCtx, bson.M{"contract_address": strings.ToLower(pAddress)}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// TokenGetByNFTIdentifiers gets tokens for a given contract address and token ID
func TokenGetByNFTIdentifiers(pCtx context.Context, pTokenID string, pAddress string,
	pRuntime *runtime.Runtime) ([]*Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}
	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

	result := []*Token{}

	err := mp.find(pCtx, bson.M{"token_id": pTokenID, "contract_address": strings.ToLower(pAddress)}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// TokenGetByID gets tokens for a given DB ID
func TokenGetByID(pCtx context.Context, pID DBID,
	pRuntime *runtime.Runtime) ([]*Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}
	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

	result := []*Token{}

	err := mp.find(pCtx, bson.M{"_id": pID}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// TokenBulkUpsert will create a bulk operation on the database to upsert many tokens for a given wallet address
// This function's primary purpose is to be used when syncing a user's tokens from an external provider
func TokenBulkUpsert(pCtx context.Context, pERC721s []*Token, pRuntime *runtime.Runtime) error {

	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

	wg := &sync.WaitGroup{}
	mu := &sync.Mutex{}
	errs := []error{}
	wg.Add(len(pERC721s))

	for _, v := range pERC721s {

		go func(token *Token) {
			defer wg.Done()
			_, err := mp.upsert(pCtx, bson.M{"token_id": token.TokenID, "contract_address": strings.ToLower(token.ContractAddress), "owner_address": token.OwnerAddress}, token)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(v)
	}
	wg.Wait()

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// TokenUpdateByID will update a given token by its DB ID and owner user ID
func TokenUpdateByID(pCtx context.Context, pID DBID, pUserID DBID,
	pUpdate interface{},
	pRuntime *runtime.Runtime) error {

	user, err := UserGetByID(pCtx, pUserID, pRuntime)
	if err != nil {
		return err
	}

	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

	return mp.update(pCtx, bson.M{"_id": pID, "owner_address": bson.M{"$in": user.Addresses}}, pUpdate)

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
