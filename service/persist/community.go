package persist

import (
	"context"
	"fmt"
)

// Community represents a community and is only cached (has no ID)
type Community struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	ContractAddress Address    `json:"contract_address"`
	CreatorAddress  Address    `json:"creator_address"`
	Name            NullString `json:"name"`
	Description     NullString `json:"description"`
	PreviewImage    NullString `json:"preview_image"`

	Owners []CommunityOwner `json:"owners"`
}

// CommunityOwner represents a user in a community
type CommunityOwner struct {
	Address  Address    `json:"address"`
	Username NullString `json:"username"`
}

// ErrCommunityNotFound is returned when a community is not found
type ErrCommunityNotFound struct {
	CommunityAddress Address
}

// CommunityRepository represents a repository for interacting with persisted communities
type CommunityRepository interface {
	GetByAddress(context.Context, Address) (Community, error)
}

func (e ErrCommunityNotFound) Error() string {
	return fmt.Sprintf("community not found: %s", e.CommunityAddress)
}
