package persist

import "context"

// MembershipTier represents the membership tier of a user
type MembershipTier struct {
	Version      int64           `bson:"version"` // schema version for this model
	ID           DBID            `bson:"_id"           json:"id" binding:"required"`
	CreationTime CreationTime    `bson:"created_at" json:"created_at"`
	Deleted      bool            `bson:"deleted" json:"-"`
	LastUpdated  LastUpdatedTime `bson:"last_updated,update_time" json:"last_updated"`

	Name     string   `bson:"name" json:"name"`
	TokenID  TokenID  `bson:"token_id" json:"token_id"`
	AssetURL string   `bson:"asset_url" json:"asset_url"`
	Owners   []string `bson:"owners" json:"owners"`
}

// MembershipRepository represents the interface for interacting with the persisted state of users
type MembershipRepository interface {
	UpsertByTokenID(context.Context, TokenID, MembershipTier) error
	GetByTokenID(context.Context, TokenID) (MembershipTier, error)
	GetAll(context.Context) ([]MembershipTier, error)
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
