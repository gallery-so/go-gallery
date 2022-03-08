package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"
)

// Community represents a community
type Community struct {
	Version         NullInt32             `json:"version"` // schema version for this model
	ID              DBID                  `json:"id" binding:"required"`
	CreationTime    CreationTime          `json:"created_at"`
	Deleted         NullBool              `json:"-"`
	LastUpdated     LastUpdatedTime       `json:"last_updated"`
	Name            NullString            `json:"name"`
	Description     NullString            `json:"description"`
	ContractAddress Address               `json:"contract_address"`
	TokenIDRanges   []TokenIDRange        `json:"token_id_ranges"`
	ProfileImageURL NullString            `json:"profile_image_url"`
	BannerImageURL  NullString            `json:"banner_image_url"`
	Owners          []CommunityTokenOwner `json:"owners"`
}

// CommunityTokenOwner represents a user who owns a community token
type CommunityTokenOwner struct {
	UserID      DBID         `json:"user_id"`
	Address     Address      `json:"address"`
	Username    NullString   `json:"username"`
	PreviewNFTs []NullString `json:"preview_nfts"`
}

// TokenIDRange represents a range of token ids
type TokenIDRange struct {
	Start TokenID
	End   TokenID
}

// CommunityRepository represents the interface for interacting with the persisted state of users
type CommunityRepository interface {
	UpsertByContract(context.Context, Address, Community) error
	GetByContract(context.Context, Address) (Community, error)
	GetAll(context.Context) ([]Community, error)
}

// Value implements the database/sql/driver Valuer interface for the membership owner type
func (o CommunityTokenOwner) Value() (driver.Value, error) {
	return json.Marshal(o)
}

// Scan implements the database/sql Scanner interface for the membership owner type
func (o *CommunityTokenOwner) Scan(src interface{}) error {
	if src == nil {
		*o = CommunityTokenOwner{}
		return nil
	}
	return json.Unmarshal(src.([]uint8), o)
}

// Value implements the database/sql/driver Valuer interface for the membership owner type
func (t TokenIDRange) Value() (driver.Value, error) {
	return json.Marshal(t)
}

// Scan implements the database/sql Scanner interface for the membership owner type
func (t *TokenIDRange) Scan(src interface{}) error {
	if src == nil {
		*t = TokenIDRange{}
		return nil
	}
	return json.Unmarshal(src.([]uint8), t)
}

// Contains returns true if the token id is within the token id range
func (t TokenIDRange) Contains(id TokenID) bool {
	return t.Start <= id && id <= t.End
}

// ErrCommunityNotFoundByAddress represents an error when a membership is not found by contract address
type ErrCommunityNotFoundByAddress struct {
	ContractAddress Address
}

func (e ErrCommunityNotFoundByAddress) Error() string {
	return "membership not found by token id: " + e.ContractAddress.String()
}
