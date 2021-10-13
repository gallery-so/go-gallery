package persist

import (
	"context"
	"time"
)

// GalleryTokenDB represents a group of collections of NFTs in the database.
// Collections of NFTs will be represented as a list of collection IDs creating
// a join relationship in the database
// This struct will only be used in database operations
type GalleryTokenDB struct {
	Version      int64     `bson:"version"       json:"version"` // schema version for this model
	ID           DBID      `bson:"_id"           json:"id" binding:"required"`
	CreationTime time.Time `bson:"created_at" json:"created_at"`
	Deleted      bool      `bson:"deleted" json:"-"`
	LastUpdated  time.Time `bson:"last_updated" json:"last_updated"`

	OwnerUserID DBID   `bson:"owner_user_id" json:"owner_user_id"`
	Collections []DBID `bson:"collections"          json:"collections"`
}

// GalleryToken represents a group of collections of NFTS in the application.
// Collections are represented as structs instead of IDs
// This struct will be decoded from a find database operation and used throughout
// the application where GalleryDB is not used
type GalleryToken struct {
	Version      int64     `bson:"version"       json:"version"` // schema version for this model
	ID           DBID      `bson:"_id"           json:"id" binding:"required"`
	CreationTime time.Time `bson:"created_at" json:"created_at"`
	Deleted      bool      `bson:"deleted" json:"-"`
	LastUpdated  time.Time `bson:"last_updated" json:"last_updated"`

	OwnerUserID DBID               `bson:"owner_user_id" json:"owner_user_id"`
	Collections []*CollectionToken `bson:"collections"          json:"collections"`
}

// GalleryTokenUpdateInput represents a struct that is used to update a gallery's list of collections in the databse
type GalleryTokenUpdateInput struct {
	Collections []DBID `bson:"collections" json:"collections"`
}

// GalleryTokenRepository is an interface for interacting with the gallery persistence layer
type GalleryTokenRepository interface {
	Create(context.Context, *GalleryTokenDB) (DBID, error)
	Update(context.Context, DBID, DBID, *GalleryTokenUpdateInput) error
	UpdateUnsafe(context.Context, DBID, *GalleryTokenUpdateInput) error
	AddCollections(context.Context, DBID, DBID, []DBID) error
	GetByUserID(context.Context, DBID, bool) ([]*GalleryToken, error)
	GetByID(context.Context, DBID, bool) ([]*GalleryToken, error)
}
