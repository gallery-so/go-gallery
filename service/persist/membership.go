package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"
)

// MembershipTier represents the membership tier of a user
type MembershipTier struct {
	Version      int64           `bson:"version"` // schema version for this model
	ID           DBID            `bson:"_id"           json:"id" binding:"required"`
	CreationTime CreationTime    `bson:"created_at" json:"created_at"`
	Deleted      bool            `bson:"deleted" json:"-"`
	LastUpdated  LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	Name     string            `bson:"name" json:"name"`
	TokenID  TokenID           `bson:"token_id" json:"token_id"`
	AssetURL string            `bson:"asset_url" json:"asset_url"`
	Owners   []MembershipOwner `bson:"owners" json:"owners"`
}

// MembershipOwner represents a user who owns a membership card
type MembershipOwner struct {
	UserID      DBID     `bson:"user_id" json:"user_id"`
	Address     Address  `bson:"address" json:"address"`
	Username    string   `bson:"username" json:"username"`
	PreviewNFTs []string `bson:"preview_nfts" json:"preview_nfts"`
}

// MembershipRepository represents the interface for interacting with the persisted state of users
type MembershipRepository interface {
	UpsertByTokenID(context.Context, TokenID, MembershipTier) error
	GetByTokenID(context.Context, TokenID) (MembershipTier, error)
	GetAll(context.Context) ([]MembershipTier, error)
}

// Value implements the database/sql/driver Valuer interface for the membership owner type
func (o MembershipOwner) Value() (driver.Value, error) {
	bs, err := json.Marshal(o)
	if err != nil {
		return nil, err
	}
	return string(bs), nil
}

// ErrMembershipNotFoundByTokenID represents an error when a membership is not found by token id
type ErrMembershipNotFoundByTokenID struct {
	TokenID TokenID
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
