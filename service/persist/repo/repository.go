package repo

import (
	"context"

	"github.com/mikeydub/go-gallery/service/persist"
)

// Repositories is the set of all available persistence repositories
type Repositories struct {
	UserRepository        UserRepository
	NonceRepository       NonceRepository
	LoginRepository       LoginAttemptRepository
	TokenRepository       TokenGalleryRepository
	CollectionRepository  CollectionRepository
	GalleryRepository     GalleryRepository
	ContractRepository    ContractGalleryRepository
	BackupRepository      BackupRepository
	MembershipRepository  MembershipRepository
	CommunityRepository   CommunityRepository
	EarlyAccessRepository EarlyAccessRepository
	WalletRepository      WalletRepository
}

// UserRepository represents the interface for interacting with the persisted state of users
type UserRepository interface {
	UpdateByID(context.Context, persist.DBID, interface{}) error
	Create(context.Context, persist.CreateUserInput) (persist.DBID, error)
	AddWallet(context.Context, persist.DBID, persist.ChainAddress, persist.WalletType) error
	RemoveWallet(context.Context, persist.DBID, persist.DBID) error
	GetByID(context.Context, persist.DBID) (persist.User, error)
	GetByWalletID(context.Context, persist.DBID) (persist.User, error)
	GetByChainAddress(context.Context, persist.ChainAddress) (persist.User, error)
	GetByUsername(context.Context, string) (persist.User, error)
	Delete(context.Context, persist.DBID) error
	MergeUsers(context.Context, persist.DBID, persist.DBID) error
	AddFollower(pCtx context.Context, follower persist.DBID, followee persist.DBID) (refollowed bool, err error)
	RemoveFollower(pCtx context.Context, follower persist.DBID, followee persist.DBID) error
	UserFollowsUser(pCtx context.Context, userA persist.DBID, userB persist.DBID) (bool, error)
}

// NonceRepository is the interface for interacting with the auth nonce persistence layer
type NonceRepository interface {
	Get(context.Context, persist.ChainAddress) (persist.UserNonce, error)
	Create(context.Context, string, persist.ChainAddress) error
}

// LoginAttemptRepository is the interface for interacting with the auth login attempt persistence layer
type LoginAttemptRepository interface {
	Create(context.Context, persist.CreateLoginAttemptInput) (persist.DBID, error)
}

// TokenRepository represents a repository for interacting with persisted tokens
type TokenRepository interface {
	CreateBulk(context.Context, []persist.Token) ([]persist.DBID, error)
	Create(context.Context, persist.Token) (persist.DBID, error)
	GetByWallet(context.Context, persist.EthereumAddress, int64, int64) ([]persist.Token, error)
	GetByContract(context.Context, persist.EthereumAddress, int64, int64) ([]persist.Token, error)
	GetByTokenIdentifiers(context.Context, persist.TokenID, persist.EthereumAddress, int64, int64) ([]persist.Token, error)
	GetByTokenID(context.Context, persist.TokenID, int64, int64) ([]persist.Token, error)
	GetByID(context.Context, persist.DBID) (persist.Token, error)
	BulkUpsert(context.Context, []persist.Token) error
	Upsert(context.Context, persist.Token) error
	UpdateByID(context.Context, persist.DBID, interface{}) error
	UpdateByTokenIdentifiers(context.Context, persist.TokenID, persist.EthereumAddress, interface{}) error
	MostRecentBlock(context.Context) (persist.BlockNumber, error)
	Count(context.Context, persist.TokenCountType) (int64, error)
}

// TokenGalleryRepository represents a repository for interacting with persisted tokens
type TokenGalleryRepository interface {
	CreateBulk(context.Context, []persist.TokenGallery) ([]persist.DBID, error)
	Create(context.Context, persist.TokenGallery) (persist.DBID, error)
	GetByUserID(context.Context, persist.DBID, int64, int64) ([]persist.TokenGallery, error)
	GetByContract(context.Context, persist.Address, persist.Chain, int64, int64) ([]persist.TokenGallery, error)
	GetByTokenIdentifiers(context.Context, persist.TokenID, persist.Address, persist.Chain, int64, int64) ([]persist.TokenGallery, error)
	GetByTokenID(context.Context, persist.TokenID, int64, int64) ([]persist.TokenGallery, error)
	GetByID(context.Context, persist.DBID) (persist.TokenGallery, error)
	BulkUpsert(context.Context, []persist.TokenGallery) error
	Upsert(context.Context, persist.TokenGallery) error
	UpdateByIDUnsafe(context.Context, persist.DBID, interface{}) error
	UpdateByID(context.Context, persist.DBID, persist.DBID, interface{}) error
	UpdateByTokenIdentifiersUnsafe(context.Context, persist.TokenID, persist.Address, persist.Chain, interface{}) error
	MostRecentBlock(context.Context) (persist.BlockNumber, error)
	Count(context.Context, persist.TokenCountType) (int64, error)
	DeleteByID(context.Context, persist.DBID) error
}

// CollectionRepository represents the interface for interacting with the collection persistence layer
type CollectionRepository interface {
	Create(context.Context, persist.CollectionDB) (persist.DBID, error)
	GetByUserID(context.Context, persist.DBID) ([]persist.Collection, error)
	GetByID(context.Context, persist.DBID) (persist.Collection, error)
	Update(context.Context, persist.DBID, persist.DBID, interface{}) error
	UpdateTokens(context.Context, persist.DBID, persist.DBID, persist.CollectionUpdateTokensInput) error
	UpdateUnsafe(context.Context, persist.DBID, interface{}) error
	UpdateNFTsUnsafe(context.Context, persist.DBID, persist.CollectionUpdateTokensInput) error
	// TODO move this to package multichain
	ClaimNFTs(context.Context, persist.DBID, []persist.EthereumAddress, persist.CollectionUpdateTokensInput) error
	RemoveNFTsOfOldAddresses(context.Context, persist.DBID) error
	// TODO move this to package multichain
	RemoveNFTsOfAddresses(context.Context, persist.DBID, []persist.EthereumAddress) error
	Delete(context.Context, persist.DBID, persist.DBID) error
}

// GalleryRepository is an interface for interacting with the gallery persistence layer
type GalleryRepository interface {
	Create(context.Context, persist.GalleryDB) (persist.DBID, error)
	Update(context.Context, persist.DBID, persist.DBID, persist.GalleryTokenUpdateInput) error
	UpdateUnsafe(context.Context, persist.DBID, persist.GalleryTokenUpdateInput) error
	AddCollections(context.Context, persist.DBID, persist.DBID, []persist.DBID) error
	GetByUserID(context.Context, persist.DBID) ([]persist.Gallery, error)
	GetByID(context.Context, persist.DBID) (persist.Gallery, error)
	RefreshCache(context.Context, persist.DBID) error
}

// ContractRepository represents a repository for interacting with persisted contracts
type ContractRepository interface {
	GetByAddress(context.Context, persist.EthereumAddress) (persist.Contract, error)
	UpsertByAddress(context.Context, persist.EthereumAddress, persist.Contract) error
	BulkUpsert(context.Context, []persist.Contract) error
}

// ContractGalleryRepository represents a repository for interacting with persisted contracts
type ContractGalleryRepository interface {
	GetByAddress(context.Context, persist.Address, persist.Chain) (persist.ContractGallery, error)
	GetByAddresses(context.Context, []persist.Address, persist.Chain) ([]persist.ContractGallery, error)
	UpsertByAddress(context.Context, persist.Address, persist.Chain, persist.ContractGallery) error
	BulkUpsert(context.Context, []persist.ContractGallery) error
}

// BackupRepository is the interface for interacting with backed up versions of galleries
type BackupRepository interface {
	Insert(context.Context, persist.Gallery) error
	Get(context.Context, persist.DBID) ([]persist.Backup, error)
	Restore(context.Context, persist.DBID, persist.DBID) error
}

// MembershipRepository represents the interface for interacting with the persisted state of users
type MembershipRepository interface {
	UpsertByTokenID(context.Context, persist.TokenID, persist.MembershipTier) error
	GetByTokenID(context.Context, persist.TokenID) (persist.MembershipTier, error)
	GetAll(context.Context) ([]persist.MembershipTier, error)
}

// CommunityRepository represents a repository for interacting with persisted communities
type CommunityRepository interface {
	GetByAddress(context.Context, persist.ChainAddress, bool) (persist.Community, error)
}

type EarlyAccessRepository interface {
	IsAllowedByAddresses(context.Context, []persist.ChainAddress) (bool, error)
}

// WalletRepository represents a repository for interacting with persisted wallets
type WalletRepository interface {
	GetByID(context.Context, persist.DBID) (persist.Wallet, error)
	GetByChainAddress(context.Context, persist.ChainAddress) (persist.Wallet, error)
	GetByUserID(context.Context, persist.DBID) ([]persist.Wallet, error)
	Insert(context.Context, persist.ChainAddress, persist.WalletType) (persist.DBID, error)
}

// EventRepository represents a repository for interacting with raw events.
type EventRepository interface {
}

// FeedRepository  represents a repository for interacting with a feed.
type FeedRepository interface {
}
