package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

const (
	minColumns     = 0
	maxColumns     = 10
	defaultColumns = 3
	maxWhitespace  = 1000
)

// CollectionDB is the struct that represents a collection of tokens in the database
// CollectionDB will not store the tokens by value but instead by ID creating a join relationship
// between collections and tokens
// This struct will only be used when updating or querying the database
type CollectionDB struct {
	Version        NullInt32                        `json:"version"` // schema version for this model
	ID             DBID                             `json:"id" binding:"required"`
	CreationTime   CreationTime                     `json:"created_at"`
	Deleted        NullBool                         `json:"-"`
	LastUpdated    LastUpdatedTime                  `json:"last_updated"`
	Layout         TokenLayout                      `json:"layout"`
	Name           NullString                       `json:"name"`
	CollectorsNote NullString                       `json:"collectors_note"`
	OwnerUserID    DBID                             `json:"owner_user_id"`
	Tokens         []DBID                           `json:"tokens"`
	Hidden         NullBool                         `json:"hidden"` // collections can be hidden from public-viewing
	TokenSettings  map[DBID]CollectionTokenSettings `json:"token_settings"`
}

// Collection represents a collection of NFTs in the application. Collection will contain
// the value of each NFT represented as a struct as opposed to just the ID of the NFT
// This struct will always be decoded from a get database operation and will be used throughout
// the application where CollectionDB does not apply
type Collection struct {
	Version        NullInt32                        `json:"version"` // schema version for this model
	ID             DBID                             `json:"id" binding:"required"`
	CreationTime   CreationTime                     `json:"created_at"`
	Deleted        NullBool                         `json:"-"`
	LastUpdated    LastUpdatedTime                  `json:"last_updated"`
	Layout         TokenLayout                      `json:"layout"`
	Name           NullString                       `json:"name"`
	CollectorsNote NullString                       `json:"collectors_note"`
	OwnerUserID    DBID                             `json:"owner_user_id"`
	NFTs           []TokenInCollection              `json:"nfts"`
	Hidden         NullBool                         `json:"hidden"` // collections can be hidden from public-viewing
	TokenSettings  map[DBID]CollectionTokenSettings `json:"token_settings"`
}

// TokenLayout defines the layout of a collection of tokens
type TokenLayout struct {
	Sections      []int                     `json:"sections"`
	SectionLayout []CollectionSectionLayout `json:"section_layout"`
	// Padding         int   `bson:"padding" json:"padding"`
}

// CollectionSectionLayout defines the layout of a section in a collection
type CollectionSectionLayout struct {
	Columns    NullInt32 `json:"columns"`
	Whitespace []int     `json:"whitespace"`
}

// CollectionUpdateInfoInput represents the data that will be changed when updating a collection's metadata
type CollectionUpdateInfoInput struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	Name           NullString `json:"name"`
	CollectorsNote NullString `json:"collectors_note"`
}

// CollectionUpdateTokensInput represents the data that will be changed when updating a collection's NFTs
type CollectionUpdateTokensInput struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	Tokens        []DBID                           `json:"tokens"`
	Layout        TokenLayout                      `json:"layout"`
	TokenSettings map[DBID]CollectionTokenSettings `json:"token_settings"`
}

// CollectionUpdateHiddenInput represents the data that will be changed when updating a collection's hidden status
type CollectionUpdateHiddenInput struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	Hidden NullBool `json:"hidden"`
}

// CollectionUpdateDeletedInput represents the data that will be changed when updating a collection's deleted status
type CollectionUpdateDeletedInput struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	Deleted NullBool `json:"-"`
}

// CollectionTokenSettings represents configurable token display options per collection
type CollectionTokenSettings struct {
	RenderLive bool `json:"render_live"`
}

// CollectionRepository represents the interface for interacting with the collection persistence layer
type CollectionRepository interface {
	Create(context.Context, CollectionDB) (DBID, error)
	GetByUserID(context.Context, DBID) ([]Collection, error)
	GetByID(context.Context, DBID) (Collection, error)
	Update(context.Context, DBID, DBID, interface{}) error
	UpdateTokens(context.Context, DBID, DBID, CollectionUpdateTokensInput) error
	UpdateUnsafe(context.Context, DBID, interface{}) error
	UpdateNFTsUnsafe(context.Context, DBID, CollectionUpdateTokensInput) error
	// TODO move this to package multichain
	ClaimNFTs(context.Context, DBID, []EthereumAddress, CollectionUpdateTokensInput) error
	RemoveNFTsOfOldAddresses(context.Context, DBID) error
	// TODO move this to package multichain
	RemoveNFTsOfAddresses(context.Context, DBID, []EthereumAddress) error
	Delete(context.Context, DBID, DBID) error
}

// ErrCollectionNotFoundByID is returned when a collection is not found by ID
type ErrCollectionNotFoundByID struct {
	ID DBID
}

// ErrInvalidLayout is returned when a layout is invalid
type ErrInvalidLayout struct {
	Layout CollectionSectionLayout
	Reason string
}

func (e ErrCollectionNotFoundByID) Error() string {
	return fmt.Sprintf("collection not found by id: %s", e.ID)
}

func (e ErrInvalidLayout) Error() string {
	return fmt.Sprintf("invalid layout: %s - %+v", e.Reason, e.Layout)
}

// ValidateLayout ensures a layout is within constraints and if has unset properties, sets their defaults
func ValidateLayout(layout TokenLayout, tokens []DBID) (TokenLayout, error) {
	layout.Sections = standardizeSections(layout.Sections)
	for i, section := range layout.SectionLayout {
		var tokenCount int
		if layout.Sections[i]+1 > len(layout.Sections) {
			tokenCount = len(layout.Sections[layout.Sections[i]:])
		} else {
			tokenCount = layout.Sections[i+1] - layout.Sections[i] + 1
		}
		validated, err := validateSectionLayout(section, tokenCount)
		if err != nil {
			return TokenLayout{}, err
		}
		layout.SectionLayout[i] = validated
	}

	return layout, nil
}

// standardizeSections formats the input sections to make it more convenient to parse.
func standardizeSections(sections []int) []int {
	if len(sections) == 0 {
		return []int{0}
	}
	if sections[0] != 0 {
		return append([]int{0}, sections...)
	}
	return sections
}

// tokensInSection returns the number of tokens in a section
func tokensInSection(sectionPos int, sections []int) int {
	if sections[sectionPos] > len(sections) {
		return len(sections[sections[sectionPos]:])
	}
	return sections[sectionPos+1] - sections[sectionPos] + 1
}

func validateSectionLayout(layout CollectionSectionLayout, sectionTokenCount int) (CollectionSectionLayout, error) {
	if layout.Columns < minColumns || layout.Columns > maxColumns {
		return CollectionSectionLayout{}, ErrInvalidLayout{
			Layout: layout,
			Reason: fmt.Sprintf("columns must be between %d-%d", minColumns, maxColumns),
		}
	}

	if layout.Columns == 0 {
		layout.Columns = defaultColumns
	}

	if ws := len(layout.Whitespace); ws > maxWhitespace {
		return CollectionSectionLayout{}, ErrInvalidLayout{
			Layout: layout,
			Reason: fmt.Sprintf("up to %d whitespace blocks permitted", maxWhitespace),
		}
	}

	for i, idx := range layout.Whitespace {
		if idx > sectionTokenCount {
			return CollectionSectionLayout{}, ErrInvalidLayout{
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

// Value implements the driver.Valuer interface for the CollectionTokenSettings type
func (s CollectionTokenSettings) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// Scan implements the Scanner interface for the CollectionTokenSettings type
func (s *CollectionTokenSettings) Scan(value interface{}) error {
	if value == nil {
		*s = CollectionTokenSettings{}
		return nil
	}

	return json.Unmarshal(value.([]byte), s)
}
