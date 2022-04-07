package persist

import "context"

// Community represents a community and is only cached (has no ID)
type Community struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	ContractAddress Address `json:"contract_address"`
	CreatorAddress  Address
	Name            NullString `json:"name"`
	Description     NullString `json:"description"`
	ProfileImageURL NullString `json:"profile_image_url"`

	Owners []CommunityOwner `json:"owners"`
}

// CommunityOwner represents a user in a community
type CommunityOwner struct {
	Address  Address    `json:"address"`
	Username NullString `json:"username"`
}

// CommunityRepository represents a repository for interacting with persisted communities
type CommunityRepository interface {
	GetByAddress(context.Context, Address) (Community, error)
}
