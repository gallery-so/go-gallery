package persist

import (
	"context"
	"fmt"
)

// Community represents a community and is only cached (has no ID)
type Community struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	ContractAddress EthereumAddress `json:"contract_address"`
	CreatorAddress  EthereumAddress `json:"creator_address"`
	Name            NullString      `json:"name"`
	Description     NullString      `json:"description"`
	PreviewImage    NullString      `json:"preview_image"`

	Owners []CommunityOwner `json:"owners"`
}

// CommunityOwner represents a user in a community
type CommunityOwner struct {
	Address  EthereumAddress `json:"address"`
	Username NullString      `json:"username"`
}

// ErrCommunityNotFound is returned when a community is not found
type ErrCommunityNotFound struct {
	CommunityAddress EthereumAddress
}

// CommunityRepository represents a repository for interacting with persisted communities
type CommunityRepository interface {
	GetByAddress(context.Context, EthereumAddress) (Community, error)
}

func (e ErrCommunityNotFound) Error() string {
	return fmt.Sprintf("community not found: %s", e.CommunityAddress)
}
