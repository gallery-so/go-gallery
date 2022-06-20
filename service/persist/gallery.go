package persist

import (
	"context"
	"fmt"
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

// GalleryTokenUpdateInput represents a struct that is used to update a gallery's list of collections in the databse
type GalleryTokenUpdateInput struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	Collections []DBID `json:"collections"`
}

// GalleryRepository is an interface for interacting with the gallery persistence layer
type GalleryRepository interface {
	Create(context.Context, GalleryDB) (DBID, error)
	Update(context.Context, DBID, DBID, GalleryTokenUpdateInput) error
	UpdateUnsafe(context.Context, DBID, GalleryTokenUpdateInput) error
	AddCollections(context.Context, DBID, DBID, []DBID) error
	GetByUserID(context.Context, DBID) ([]Gallery, error)
	GetByID(context.Context, DBID) (Gallery, error)
	RefreshCache(context.Context, DBID) error
}

// ErrGalleryNotFoundByID is returned when a gallery is not found by its ID
type ErrGalleryNotFoundByID struct {
	ID DBID
}

func (e ErrGalleryNotFoundByID) Error() string {
	return fmt.Sprintf("gallery not found with ID: %v", e.ID)
}

// ErrGalleryNotFoundByCollectionID is returned when a gallery is not found by a child collection ID
type ErrGalleryNotFoundByCollectionID struct {
	ID DBID
}

func (e ErrGalleryNotFoundByCollectionID) Error() string {
	return fmt.Sprintf("gallery not found for child collection ID: %v", e.ID)
}
