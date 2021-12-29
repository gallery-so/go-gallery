package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"
)

// GalleryDB represents a group of collections of NFTs in the database.
// Collections of NFTs will be represented as a list of collection IDs creating
// a join relationship in the database
// This struct will only be used in database operations
type GalleryDB struct {
	Version      int64           `bson:"version"       json:"version"` // schema version for this model
	ID           DBID            `bson:"_id"           json:"id" binding:"required"`
	CreationTime CreationTime    `bson:"created_at" json:"created_at"`
	Deleted      bool            `bson:"deleted" json:"-"`
	LastUpdated  LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	OwnerUserID DBID   `bson:"owner_user_id" json:"owner_user_id"`
	Collections []DBID `bson:"collections"          json:"collections"`
}

// Gallery represents a group of collections of NFTS in the application.
// Collections are represented as structs instead of IDs
// This struct will be decoded from a find database operation and used throughout
// the application where GalleryDB is not used
type Gallery struct {
	Version      int64           `bson:"version"       json:"version"` // schema version for this model
	ID           DBID            `bson:"_id"           json:"id" binding:"required"`
	CreationTime CreationTime    `bson:"created_at" json:"created_at"`
	Deleted      bool            `bson:"deleted" json:"-"`
	LastUpdated  LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	OwnerUserID DBID         `bson:"owner_user_id" json:"owner_user_id"`
	Collections []Collection `bson:"collections"          json:"collections"`
}

// GalleryUpdateInput represents a struct that is used to update a gallery's list of collections in the databse
type GalleryUpdateInput struct {
	LastUpdated LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	Collections []DBID `bson:"collections" json:"collections"`
}

// GalleryRepository is an interface for interacting with the gallery persistence layer
type GalleryRepository interface {
	Create(context.Context, GalleryDB) (DBID, error)
	Update(context.Context, DBID, DBID, GalleryUpdateInput) error
	AddCollections(context.Context, DBID, DBID, []DBID) error
	GetByUserID(context.Context, DBID) ([]Gallery, error)
	GetByID(context.Context, DBID) (Gallery, error)
	RefreshCache(context.Context, DBID) error
}

// Value implements the driver.Valuer interface for Gallery
func (g Gallery) Value() (driver.Value, error) {
	return json.Marshal(g)
}
