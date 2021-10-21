package persist

import (
	"context"
	"fmt"
	"time"
)

// CollectionTokenDB is the struct that represents a collection of NFTs in the database
// CollectionTokenDB will not store the NFTs by value but instead by ID creating a join relationship
// between collections and NFTS
// This struct will only be used when updating or querying the database
type CollectionTokenDB struct {
	Version      int64     `bson:"version" json:"version"` // schema version for this model
	ID           DBID      `bson:"_id" json:"id" binding:"required"`
	CreationTime time.Time `bson:"created_at" json:"created_at"`
	Deleted      bool      `bson:"deleted" json:"-"`
	LastUpdated  time.Time `bson:"last_updated" json:"last_updated"`

	GalleryID DBID `bson:"gallery_id" json:"gallery_id"`

	Name           string `bson:"name"          json:"name"`
	CollectorsNote string `bson:"collectors_note"   json:"collectors_note"`
	OwnerUserID    DBID   `bson:"owner_user_id" json:"owner_user_id"`
	Nfts           []DBID `bson:"nfts"          json:"nfts"`

	// collections can be hidden from public-viewing
	Hidden bool `bson:"hidden" json:"hidden"`
}

// CollectionToken represents a collection of NFTs in the application. CollectionToken will contain
// the value of each NFT represented as a struct as opposed to just the ID of the NFT
// This struct will always be decoded from a get database operation and will be used throughout
// the application where CollectionDB does not apply
type CollectionToken struct {
	Version      int64     `bson:"version"       json:"version"` // schema version for this model
	ID           DBID      `bson:"_id"           json:"id" binding:"required"`
	CreationTime time.Time `bson:"created_at" json:"created_at"`
	Deleted      bool      `bson:"deleted" json:"-"`
	LastUpdated  time.Time `bson:"last_updated" json:"last_updated"`

	Name           string               `bson:"name"          json:"name"`
	CollectorsNote string               `bson:"collectors_note"   json:"collectors_note"`
	OwnerUserID    string               `bson:"owner_user_id" json:"owner_user_id"`
	Nfts           []*TokenInCollection `bson:"nfts"          json:"nfts"`

	// collections can be hidden from public-viewing
	Hidden bool `bson:"hidden" json:"hidden"`
}

// CollectionTokenUpdateInfoInput represents the data that will be changed when updating a collection's metadata
type CollectionTokenUpdateInfoInput struct {
	Name           string `bson:"name" json:"name"`
	CollectorsNote string `bson:"collectors_note" json:"collectors_note"`
}

// CollectionTokenUpdateNftsInput represents the data that will be changed when updating a collection's NFTs
type CollectionTokenUpdateNftsInput struct {
	Nfts []DBID `bson:"nfts" json:"nfts"`
}

// CollectionTokenUpdateHiddenInput represents the data that will be changed when updating a collection's hidden status
type CollectionTokenUpdateHiddenInput struct {
	Hidden bool `bson:"hidden" json:"hidden"`
}

// CollectionTokenUpdateDeletedInput represents the data that will be changed when updating a collection's deleted status
type CollectionTokenUpdateDeletedInput struct {
	Deleted bool `bson:"deleted" json:"-"`
}

// CollectionTokenRepository represents the interface for interacting with the collection persistence layer
type CollectionTokenRepository interface {
	Create(context.Context, *CollectionTokenDB) (DBID, error)
	GetByUserID(context.Context, DBID, bool) ([]*CollectionToken, error)
	GetByID(context.Context, DBID, bool) (*CollectionToken, error)
	Update(context.Context, DBID, DBID, interface{}) error
	UpdateNFTs(context.Context, DBID, DBID, *CollectionTokenUpdateNftsInput) error
	UpdateUnsafe(context.Context, DBID, interface{}) error
	UpdateNFTsUnsafe(context.Context, DBID, *CollectionTokenUpdateNftsInput) error
	ClaimNFTs(context.Context, DBID, []Address, *CollectionTokenUpdateNftsInput) error
	RemoveNFTsOfAddresses(context.Context, DBID, []Address) error
	Delete(context.Context, DBID, DBID) error
	GetUnassigned(context.Context, DBID) (*CollectionToken, error)
	RefreshUnassigned(context.Context, DBID) error
}

// ErrCollectionNotFoundByID is returned when a collection is not found by ID
type ErrCollectionNotFoundByID struct {
	ID DBID
}

func (e ErrCollectionNotFoundByID) Error() string {
	return fmt.Sprintf("collection not found by id: %s", e.ID)
}
