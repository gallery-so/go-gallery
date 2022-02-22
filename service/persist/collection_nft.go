package persist

import (
	"context"
)

// CollectionDB is the struct that represents a collection of NFTs in the database
// CollectionDB will not store the NFTs by value but instead by ID creating a join relationship
// between collections and NFTS
// This struct will only be used when updating or querying the database
type CollectionDB struct {
	Version      NullInt32       `bson:"version" json:"version"` // schema version for this model
	ID           DBID            `bson:"_id" json:"id" binding:"required"`
	CreationTime CreationTime    `bson:"created_at" json:"created_at"`
	Deleted      NullBool        `bson:"deleted" json:"-"`
	LastUpdated  LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	Layout TokenLayout `bson:"layout" json:"layout"`

	Name           NullString `bson:"name"          json:"name"`
	CollectorsNote NullString `bson:"collectors_note"   json:"collectors_note"`
	OwnerUserID    DBID       `bson:"owner_user_id" json:"owner_user_id"`
	NFTs           []DBID     `bson:"nfts"          json:"nfts"`

	// collections can be hidden from public-viewing
	Hidden NullBool `bson:"hidden" json:"hidden"`
}

// Collection represents a collection of NFTs in the application. Collection will contain
// the value of each NFT represented as a struct as opposed to just the ID of the NFT
// This struct will always be decoded from a get database operation and will be used throughout
// the application where CollectionDB does not apply
type Collection struct {
	Version      NullInt32       `bson:"version"       json:"version"` // schema version for this model
	ID           DBID            `bson:"_id"           json:"id" binding:"required"`
	CreationTime CreationTime    `bson:"created_at" json:"created_at"`
	Deleted      NullBool        `bson:"deleted" json:"-"`
	LastUpdated  LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	Layout TokenLayout `bson:"layout" json:"layout"`

	Name           NullString      `bson:"name"          json:"name"`
	CollectorsNote NullString      `bson:"collectors_note"   json:"collectors_note"`
	OwnerUserID    DBID            `bson:"owner_user_id" json:"owner_user_id"`
	NFTs           []CollectionNFT `bson:"nfts"          json:"nfts"`

	// collections can be hidden from public-viewing
	Hidden NullBool `bson:"hidden" json:"hidden"`
}

// CollectionUpdateInfoInput represents the data that will be changed when updating a collection's metadata
type CollectionUpdateInfoInput struct {
	LastUpdated LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	Name           NullString `bson:"name" json:"name" postgres:"NAME"`
	CollectorsNote NullString `bson:"collectors_note" json:"collectors_note"`
}

// CollectionUpdateNftsInput represents the data that will be changed when updating a collection's NFTs
type CollectionUpdateNftsInput struct {
	LastUpdated LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	NFTs   []DBID      `bson:"nfts" json:"nfts"`
	Layout TokenLayout `bson:"layout" json:"layout"`
}

// CollectionUpdateHiddenInput represents the data that will be changed when updating a collection's hidden status
type CollectionUpdateHiddenInput struct {
	LastUpdated LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	Hidden NullBool `bson:"hidden" json:"hidden" postgres:"HIDDEN"`
}

// CollectionUpdateDeletedInput represents the data that will be changed when updating a collection's deleted status
type CollectionUpdateDeletedInput struct {
	LastUpdated LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	Deleted NullBool `bson:"deleted" json:"-"`
}

// CollectionRepository represents the interface for interacting with the collection persistence layer
type CollectionRepository interface {
	Create(context.Context, CollectionDB) (DBID, error)
	GetByUserID(context.Context, DBID, bool) ([]Collection, error)
	GetByID(context.Context, DBID, bool) (Collection, error)
	Update(context.Context, DBID, DBID, interface{}) error
	UpdateNFTs(context.Context, DBID, DBID, CollectionUpdateNftsInput) error
	ClaimNFTs(context.Context, DBID, []Address, CollectionUpdateNftsInput) error
	RemoveNFTsOfAddresses(context.Context, DBID, []Address) error
	RemoveNFTsOfOldAddresses(context.Context, DBID) error
	Delete(context.Context, DBID, DBID) error
	GetUnassigned(context.Context, DBID) (Collection, error)
	RefreshUnassigned(context.Context, DBID) error
}
