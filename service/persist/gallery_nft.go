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
	Version      NullInt32       `json:"version"` // schema version for this model
	ID           DBID            `json:"id" binding:"required"`
	CreationTime CreationTime    `json:"created_at"`
	Deleted      NullBool        `json:"-"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`

	OwnerUserID DBID   `json:"owner_user_id"`
	Collections []DBID `json:"collections"`
}

// Gallery represents a group of collections of NFTS in the application.
// Collections are represented as structs instead of IDs
// This struct will be decoded from a find database operation and used throughout
// the application where GalleryDB is not used
type Gallery struct {
	Version      NullInt32       `json:"version"` // schema version for this model
	ID           DBID            `json:"id" binding:"required"`
	CreationTime CreationTime    `json:"created_at"`
	Deleted      NullBool        `json:"-"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`

	OwnerUserID DBID         `json:"owner_user_id"`
	Collections []Collection `json:"collections"`
}

// GalleryUpdateInput represents a struct that is used to update a gallery's list of collections in the databse
type GalleryUpdateInput struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	Collections []DBID `json:"collections"`
}

// GalleryRepository is an interface for interacting with the gallery persistence layer
type GalleryRepository interface {
	Create(context.Context, GalleryDB) (DBID, error)
	Update(context.Context, DBID, DBID, GalleryUpdateInput) error
	AddCollections(context.Context, DBID, DBID, []DBID) error
	GetByUserID(context.Context, DBID) ([]Gallery, error)
	GetByID(context.Context, DBID) (Gallery, error)
	GetByIDRaw(context.Context, DBID) (Gallery, error)
	GetByChildCollectionIDRaw(context.Context, DBID) (Gallery, error)
	RefreshCache(context.Context, DBID) error
}

// Value implements the driver.Valuer interface for Gallery
func (g Gallery) Value() (driver.Value, error) {
	return json.Marshal(g)
}

// Scan implements the sql.Scanner interface for Gallery
func (g *Gallery) Scan(value interface{}) error {
	b, ok := value.([]uint8)
	if !ok {
		return nil
	}

	return json.Unmarshal(b, g)
}
