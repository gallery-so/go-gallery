package persist

import (
	"context"
	"fmt"
)

// ContractGallery represents a smart contract in the database
type ContractGallery struct {
	Version      NullInt32       `json:"version"` // schema version for this model
	ID           DBID            `json:"id" binding:"required"`
	CreationTime CreationTime    `json:"created_at"`
	Deleted      NullBool        `json:"-"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`

	Chain            Chain      `json:"chain"`
	Address          Address    `json:"address"`
	Symbol           NullString `json:"symbol"`
	Name             NullString `json:"name"`
	Description      NullString `json:"description"`
	OwnerAddress     Address    `json:"owner_address"`
	ProfileImageURL  NullString `json:"profile_image_url"`
	ProfileBannerURL NullString `json:"profile_banner_url"`
	BadgeURL         NullString `json:"badge_url"`
}

// ErrContractNotFoundByAddress is an error type for when a contract is not found by address
type ErrGalleryContractNotFound struct {
	Address Address
	Chain   Chain
}

// ContractGalleryRepository represents a repository for interacting with persisted contracts
type ContractGalleryRepository interface {
	GetByID(ctx context.Context, id DBID) (ContractGallery, error)
	GetByAddress(context.Context, Address, Chain) (ContractGallery, error)
	GetByAddresses(context.Context, []Address, Chain) ([]ContractGallery, error)
	UpsertByAddress(context.Context, Address, Chain, ContractGallery) error
	BulkUpsert(context.Context, []ContractGallery) error
	GetOwnersByAddress(context.Context, Address, Chain, int, int) ([]TokenHolder, error)
}

func (c ContractGallery) ContractIdentifiers() ContractIdentifiers {
	return NewContractIdentifiers(c.Address, c.Chain)
}

func (e ErrGalleryContractNotFound) Error() string {
	return fmt.Sprintf("contract not found by address: %s-%d", e.Address, e.Chain)
}
