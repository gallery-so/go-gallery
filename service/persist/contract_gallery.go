package persist

import (
	"context"
	"time"
)

// ContractGallery represents a smart contract in the database
type ContractGallery struct {
	Version      NullInt32 `json:"version"` // schema version for this model
	ID           DBID      `json:"id" binding:"required"`
	ParentID     DBID      `json:"parent_id"`
	CreationTime time.Time `json:"created_at"`
	Deleted      NullBool  `json:"-"`
	LastUpdated  time.Time `json:"last_updated"`

	Chain                 Chain      `json:"chain"`
	Address               Address    `json:"address"`
	Symbol                NullString `json:"symbol"`
	Name                  NullString `json:"name"`
	Description           NullString `json:"description"`
	OwnerAddress          Address    `json:"owner_address"`
	CreatorAddress        Address    `json:"creator_address"`
	ProfileImageURL       NullString `json:"profile_image_url"`
	ProfileBannerURL      NullString `json:"profile_banner_url"`
	BadgeURL              NullString `json:"badge_url"`
	IsProviderMarkedSpam  bool       `json:"is_provider_marked_spam"`
	OverrideCreatorUserID DBID       `json:"override_creator_user_id"`
}

// ContractGalleryRepository represents a repository for interacting with persisted contracts
type ContractGalleryRepository interface {
	GetByID(ctx context.Context, id DBID) (ContractGallery, error)
	GetByAddress(context.Context, Address, Chain) (ContractGallery, error)
	UpsertByAddress(context.Context, Address, Chain, ContractGallery) error
	BulkUpsert(context.Context, []ContractGallery, bool) ([]ContractGallery, error)
	GetOwnersByAddress(context.Context, Address, Chain, int, int) ([]TokenHolder, error)
}

func (c ContractGallery) ContractIdentifiers() ContractIdentifiers {
	return NewContractIdentifiers(c.Address, c.Chain)
}
