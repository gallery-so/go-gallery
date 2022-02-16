package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

const (
	MIN_COLUMNS     = 0
	MAX_COLUMNS     = 6
	DEFAULT_COLUMNS = 3
	MAX_WHITESPACE  = 1000
)

// CollectionTokenDB is the struct that represents a collection of NFTs in the database
// CollectionTokenDB will not store the NFTs by value but instead by ID creating a join relationship
// between collections and NFTS
// This struct will only be used when updating or querying the database
type CollectionTokenDB struct {
	Version      NullInt64       `bson:"version" json:"version"` // schema version for this model
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

// CollectionToken represents a collection of NFTs in the application. CollectionToken will contain
// the value of each NFT represented as a struct as opposed to just the ID of the NFT
// This struct will always be decoded from a get database operation and will be used throughout
// the application where CollectionDB does not apply
type CollectionToken struct {
	Version      NullInt64       `bson:"version"       json:"version"` // schema version for this model
	ID           DBID            `bson:"_id"           json:"id" binding:"required"`
	CreationTime CreationTime    `bson:"created_at" json:"created_at"`
	Deleted      NullBool        `bson:"deleted" json:"-"`
	LastUpdated  LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	Layout TokenLayout `bson:"layout" json:"layout"`

	Name           NullString          `bson:"name"          json:"name"`
	CollectorsNote NullString          `bson:"collectors_note"   json:"collectors_note"`
	OwnerUserID    DBID                `bson:"owner_user_id" json:"owner_user_id"`
	NFTs           []TokenInCollection `bson:"nfts"          json:"nfts"`

	// collections can be hidden from public-viewing
	Hidden NullBool `bson:"hidden" json:"hidden"`
}

// TokenLayout defines the layout of a collection of tokens
type TokenLayout struct {
	Columns    NullInt64 `bson:"columns" json:"columns"`
	Whitespace []int     `bson:"whitespace" json:"whitespace"`
	// Padding         int   `bson:"padding" json:"padding"`
}

// CollectionTokenUpdateInfoInput represents the data that will be changed when updating a collection's metadata
type CollectionTokenUpdateInfoInput struct {
	LastUpdated LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	Name           NullString `bson:"name" json:"name"`
	CollectorsNote NullString `bson:"collectors_note" json:"collectors_note"`
}

// CollectionTokenUpdateNftsInput represents the data that will be changed when updating a collection's NFTs
type CollectionTokenUpdateNftsInput struct {
	LastUpdated LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	NFTs   []DBID      `bson:"nfts" json:"nfts"`
	Layout TokenLayout `bson:"layout" json:"layout"`
}

// CollectionTokenUpdateHiddenInput represents the data that will be changed when updating a collection's hidden status
type CollectionTokenUpdateHiddenInput struct {
	LastUpdated LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	Hidden NullBool `bson:"hidden" json:"hidden"`
}

// CollectionTokenUpdateDeletedInput represents the data that will be changed when updating a collection's deleted status
type CollectionTokenUpdateDeletedInput struct {
	LastUpdated LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	Deleted NullBool `bson:"deleted" json:"-"`
}

// CollectionTokenRepository represents the interface for interacting with the collection persistence layer
type CollectionTokenRepository interface {
	Create(context.Context, CollectionTokenDB) (DBID, error)
	GetByUserID(context.Context, DBID, bool) ([]CollectionToken, error)
	GetByID(context.Context, DBID, bool) (CollectionToken, error)
	Update(context.Context, DBID, DBID, interface{}) error
	UpdateNFTs(context.Context, DBID, DBID, CollectionTokenUpdateNftsInput) error
	UpdateUnsafe(context.Context, DBID, interface{}) error
	UpdateNFTsUnsafe(context.Context, DBID, CollectionTokenUpdateNftsInput) error
	ClaimNFTs(context.Context, DBID, []Address, CollectionTokenUpdateNftsInput) error
	RemoveNFTsOfOldAddresses(context.Context, DBID) error
	RemoveNFTsOfAddresses(context.Context, DBID, []Address) error
	Delete(context.Context, DBID, DBID) error
	GetUnassigned(context.Context, DBID) (CollectionToken, error)
	RefreshUnassigned(context.Context, DBID) error
}

// ErrCollectionNotFoundByID is returned when a collection is not found by ID
type ErrCollectionNotFoundByID struct {
	ID DBID
}

// ErrInvalidLayout is returned when a layout is invalid
type ErrInvalidLayout struct {
	Layout TokenLayout
	Reason string
}

func (e ErrCollectionNotFoundByID) Error() string {
	return fmt.Sprintf("collection not found by id: %s", e.ID)
}

func (e ErrInvalidLayout) Error() string {
	return fmt.Sprintf("invalid layout: %s - %+v", e.Reason, e.Layout)
}

// ValidateLayout ensures a layout is within constraints and if has unset properties, sets their defaults
func ValidateLayout(layout TokenLayout, nfts []DBID) (TokenLayout, error) {
	if layout.Columns < MIN_COLUMNS || layout.Columns > MAX_COLUMNS {
		return TokenLayout{}, ErrInvalidLayout{
			Layout: layout,
			Reason: fmt.Sprintf("columns must be between %d-%d", MIN_COLUMNS, MAX_COLUMNS),
		}
	}
	if layout.Columns == 0 {
		layout.Columns = DEFAULT_COLUMNS
	}

	if ws := len(layout.Whitespace); ws > MAX_WHITESPACE {
		return TokenLayout{}, ErrInvalidLayout{
			Layout: layout,
			Reason: fmt.Sprintf("up to %d whitespace blocks permitted", MAX_WHITESPACE),
		}
	}

	for i, idx := range layout.Whitespace {
		if idx > len(nfts) {
			return TokenLayout{}, ErrInvalidLayout{
				Layout: layout,
				Reason: fmt.Sprintf("position of whitespace at %d is invalid: %d", i, idx),
			}
		}
	}

	return layout, nil
}

// Value implements the driver.Valuer interface for the TokenLayout type
func (l TokenLayout) Value() (driver.Value, error) {
	return json.Marshal(l)
}

// Scan implements the Scanner interface for the TokenLayout type
func (l *TokenLayout) Scan(value interface{}) error {
	if value == nil {
		*l = TokenLayout{}
		return nil
	}
	return json.Unmarshal(value.([]uint8), l)
}
