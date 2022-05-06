package persist

import (
	"context"
)

// CollectionDB is the struct that represents a collection of NFTs in the database
// CollectionDB will not store the NFTs by value but instead by ID creating a join relationship
// between collections and NFTS
// This struct will only be used when updating or querying the database
type CollectionDB struct {
	Version      NullInt32       `json:"version"` // schema version for this model
	ID           DBID            `json:"id" binding:"required"`
	CreationTime CreationTime    `json:"created_at"`
	Deleted      NullBool        `json:"-"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`

	Layout TokenLayout `json:"layout"`

	Name           NullString `json:"name"`
	CollectorsNote NullString `json:"collectors_note"`
	OwnerUserID    DBID       `json:"owner_user_id"`
	NFTs           []DBID     `json:"nfts"`

	// collections can be hidden from public-viewing
	Hidden NullBool `json:"hidden"`
}

// Collection represents a collection of NFTs in the application. Collection will contain
// the value of each NFT represented as a struct as opposed to just the ID of the NFT
// This struct will always be decoded from a get database operation and will be used throughout
// the application where CollectionDB does not apply
type Collection struct {
	Version      NullInt32       `json:"version"` // schema version for this model
	ID           DBID            `json:"id" binding:"required"`
	CreationTime CreationTime    `json:"created_at"`
	Deleted      NullBool        `json:"-"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`

	Layout TokenLayout `json:"layout"`

	Name           NullString      `json:"name"`
	CollectorsNote NullString      `json:"collectors_note"`
	OwnerUserID    DBID            `json:"owner_user_id"`
	NFTs           []CollectionNFT `json:"nfts"`

	// collections can be hidden from public-viewing
	Hidden NullBool `json:"hidden"`
}

// CollectionUpdateInfoInput represents the data that will be changed when updating a collection's metadata
type CollectionUpdateInfoInput struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	Name           NullString `json:"name" postgres:"NAME"`
	CollectorsNote NullString `json:"collectors_note"`
}

// CollectionUpdateNftsInput represents the data that will be changed when updating a collection's NFTs
type CollectionUpdateNftsInput struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	NFTs   []DBID      `json:"nfts"`
	Layout TokenLayout `json:"layout"`
}

// CollectionUpdateHiddenInput represents the data that will be changed when updating a collection's hidden status
type CollectionUpdateHiddenInput struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	Hidden NullBool `json:"hidden" postgres:"HIDDEN"`
}

// CollectionUpdateDeletedInput represents the data that will be changed when updating a collection's deleted status
type CollectionUpdateDeletedInput struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	Deleted NullBool `json:"-"`
}

// CollectionRepository represents the interface for interacting with the collection persistence layer
type CollectionRepository interface {
	Create(context.Context, CollectionDB) (DBID, error)
	GetByUserID(context.Context, DBID, bool) ([]Collection, error)
	GetByGalleryIDRaw(context.Context, DBID, bool) ([]Collection, error)
	GetByID(context.Context, DBID, bool) (Collection, error)
	GetByIDRaw(context.Context, DBID, bool) (Collection, error)
	Update(context.Context, DBID, DBID, interface{}) error
	UpdateNFTs(context.Context, DBID, DBID, CollectionUpdateNftsInput) error
	// TODO move this to package multichain
	ClaimNFTs(context.Context, DBID, []EthereumAddress, CollectionUpdateNftsInput) error
	RemoveNFTsOfAddresses(context.Context, DBID, []EthereumAddress) error
	// TODO move this to package multichain
	RemoveNFTsOfOldAddresses(context.Context, DBID) error
	Delete(context.Context, DBID, DBID) error
}
