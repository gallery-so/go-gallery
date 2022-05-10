package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"

	"github.com/lib/pq"
)

// MembershipTier represents the membership tier of a user
type MembershipTier struct {
	Version      NullInt32       `json:"version"` // schema version for this model
	ID           DBID            `json:"id" binding:"required"`
	CreationTime CreationTime    `json:"created_at"`
	Deleted      NullBool        `json:"-"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`
	Name         NullString      `json:"name"`
	TokenID      TokenID         `json:"token_id"`
	AssetURL     NullString      `json:"asset_url"`
	Owners       []TokenHolder   `json:"owners"`
}

// TokenHolder represents a user who owns a membership card
type TokenHolder struct {
	UserID      DBID         `json:"user_id"`
	Addresses   []Address    `json:"addresses"`
	Username    NullString   `json:"username"`
	PreviewNFTs []NullString `json:"preview_nfts"`
}

// TokenHolderList is a slice of MembershipOwners, used to implement scanner/valuer interfaces
type TokenHolderList []TokenHolder

func (l TokenHolderList) Value() (driver.Value, error) {
	return pq.Array(l).Value()
}

func (l *TokenHolderList) Scan(value interface{}) error {
	return pq.Array(l).Scan(value)
}

// MembershipRepository represents the interface for interacting with the persisted state of users
type MembershipRepository interface {
	UpsertByTokenID(context.Context, TokenID, MembershipTier) error
	GetByTokenID(context.Context, TokenID) (MembershipTier, error)
	GetAll(context.Context) ([]MembershipTier, error)
}

// Value implements the database/sql/driver Valuer interface for the membership owner type
func (o TokenHolder) Value() (driver.Value, error) {
	return json.Marshal(o)
}

// Scan implements the database/sql Scanner interface for the membership owner type
func (o *TokenHolder) Scan(src interface{}) error {
	if src == nil {
		*o = TokenHolder{}
		return nil
	}
	return json.Unmarshal(src.([]uint8), o)
}

// ErrMembershipNotFoundByTokenID represents an error when a membership is not found by token id
type ErrMembershipNotFoundByTokenID struct {
	TokenID TokenID
}

// ErrMembershipNotFoundByID represents an error when a membership is not found by its id
type ErrMembershipNotFoundByID struct {
	ID DBID
}

// ErrMembershipNotFoundByName represents an error when a membership is not found by name
type ErrMembershipNotFoundByName struct {
	Name string
}

func (e ErrMembershipNotFoundByName) Error() string {
	return "membership not found by name: " + e.Name
}

func (e ErrMembershipNotFoundByTokenID) Error() string {
	return "membership not found by token id: " + e.TokenID.String()
}

func (e ErrMembershipNotFoundByID) Error() string {
	return "membership not found by id: " + e.ID.String()
}
