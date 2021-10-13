package persist

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CollectionDB is the struct that represents a collection of NFTs in the database
// CollectionDB will not store the NFTs by value but instead by ID creating a join relationship
// between collections and NFTS
// This struct will only be used when updating or querying the database
type CollectionDB struct {
	Version      int64     `bson:"version" json:"version"` // schema version for this model
	ID           DBID      `bson:"_id" json:"id" binding:"required"`
	CreationTime time.Time `bson:"created_at" json:"created_at"`
	Deleted      bool      `bson:"deleted" json:"-"`

	Name           string `bson:"name"          json:"name"`
	CollectorsNote string `bson:"collectors_note"   json:"collectors_note"`
	OwnerUserID    DBID   `bson:"owner_user_id" json:"owner_user_id"`
	Nfts           []DBID `bson:"nfts"          json:"nfts"`

	// collections can be hidden from public-viewing
	Hidden bool `bson:"hidden" json:"hidden"`
}

// Collection represents a collection of NFTs in the application. Collection will contain
// the value of each NFT represented as a struct as opposed to just the ID of the NFT
// This struct will always be decoded from a get database operation and will be used throughout
// the application where CollectionDB does not apply
type Collection struct {
	Version      int64              `bson:"version"       json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"           json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at" json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`
	LastUpdated  primitive.DateTime `bson:"last_updated" json:"last_updated"`

	Name           string           `bson:"name"          json:"name"`
	CollectorsNote string           `bson:"collectors_note"   json:"collectors_note"`
	OwnerUserID    string           `bson:"owner_user_id" json:"owner_user_id"`
	Nfts           []*CollectionNFT `bson:"nfts"          json:"nfts"`

	// collections can be hidden from public-viewing
	Hidden bool `bson:"hidden" json:"hidden"`
}

// CollectionUpdateInfoInput represents the data that will be changed when updating a collection's metadata
type CollectionUpdateInfoInput struct {
	Name           string `bson:"name" json:"name"`
	CollectorsNote string `bson:"collectors_note" json:"collectors_note"`
}

// CollectionUpdateNftsInput represents the data that will be changed when updating a collection's NFTs
type CollectionUpdateNftsInput struct {
	Nfts []DBID `bson:"nfts" json:"nfts"`
}

// CollectionUpdateHiddenInput represents the data that will be changed when updating a collection's hidden status
type CollectionUpdateHiddenInput struct {
	Hidden bool `bson:"hidden" json:"hidden"`
}

// CollectionUpdateDeletedInput represents the data that will be changed when updating a collection's deleted status
type CollectionUpdateDeletedInput struct {
	Deleted bool `bson:"deleted" json:"-"`
}

// CollectionRepository represents the interface for interacting with the collection persistence layer
type CollectionRepository interface {
	Create(context.Context, *CollectionDB) (DBID, error)
	GetByUserID(context.Context, DBID, bool) ([]*Collection, error)
	GetByID(context.Context, DBID, bool) ([]*Collection, error)
	Update(context.Context, DBID, DBID, interface{}) error
	UpdateNFTs(context.Context, DBID, DBID, *CollectionUpdateNftsInput) error
	ClaimNFTs(context.Context, DBID, []string, *CollectionUpdateNftsInput) error
	RemoveNFTsOfAddresses(context.Context, DBID, []string) error
	Delete(context.Context, DBID, DBID) error
	GetUnassigned(context.Context, DBID, bool) (*Collection, error)
}
