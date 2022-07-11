package persist

import (
	"context"
	"fmt"
)

// User represents a user with all of their addresses
type User struct {
	Version            NullInt32       `json:"version"` // schema version for this model
	ID                 DBID            `json:"id" binding:"required"`
	CreationTime       CreationTime    `json:"created_at"`
	Deleted            NullBool        `json:"-"`
	LastUpdated        LastUpdatedTime `json:"last_updated"`
	Username           NullString      `json:"username"` // mutable
	UsernameIdempotent NullString      `json:"username_idempotent"`
	Wallets            []Wallet        `json:"wallets"`
	Bio                NullString      `json:"bio"`
}

// UserUpdateInfoInput represents the data to be updated when updating a user
type UserUpdateInfoInput struct {
	LastUpdated        LastUpdatedTime `json:"last_updated"`
	Username           NullString      `json:"username"`
	UsernameIdempotent NullString      `json:"username_idempotent"`
	Bio                NullString      `json:"bio"`
}

type CreateUserInput struct {
	Username     string
	Bio          string
	ChainAddress ChainAddress
	WalletType   WalletType
}

// UserRepository represents the interface for interacting with the persisted state of users
type UserRepository interface {
	UpdateByID(context.Context, DBID, interface{}) error
	Create(context.Context, CreateUserInput) (DBID, error)
	AddWallet(context.Context, DBID, ChainAddress, WalletType) error
	RemoveWallet(context.Context, DBID, DBID) error
	GetByID(context.Context, DBID) (User, error)
	GetByWalletID(context.Context, DBID) (User, error)
	GetByChainAddress(context.Context, ChainAddress) (User, error)
	GetByUsername(context.Context, string) (User, error)
	Delete(context.Context, DBID) error
	MergeUsers(context.Context, DBID, DBID) error
	AddFollower(pCtx context.Context, follower DBID, followee DBID) (refollowed bool, err error)
	RemoveFollower(pCtx context.Context, follower DBID, followee DBID) error
	UserFollowsUser(pCtx context.Context, userA DBID, userB DBID) (bool, error)
}

// ErrUserNotFound is returned when a user is not found
type ErrUserNotFound struct {
	UserID        DBID
	WalletID      DBID
	ChainAddress  ChainAddress
	Username      string
	Authenticator string
}

func (e ErrUserNotFound) Error() string {
	return fmt.Sprintf("user not found: address: %s, ID: %s, walletID: %s, username: %s, authenticator: %s", e.ChainAddress, e.UserID, e.WalletID, e.Username, e.Authenticator)
}

type ErrUserAlreadyExists struct {
	ChainAddress  ChainAddress
	Authenticator string
	Username      string
}

func (e ErrUserAlreadyExists) Error() string {
	return fmt.Sprintf("user already exists: username: %s, address: %s, authenticator: %s", e.Username, e.ChainAddress, e.Authenticator)
}

type ErrUsernameNotAvailable struct {
	Username string
}

func (e ErrUsernameNotAvailable) Error() string {
	return fmt.Sprintf("username not available: %s", e.Username)
}

type ErrAddressOwnedByUser struct {
	ChainAddress ChainAddress
	OwnerID      DBID
}

func (e ErrAddressOwnedByUser) Error() string {
	return fmt.Sprintf("address is owned by user: address: %s, ownerID: %s", e.ChainAddress, e.OwnerID)
}

type ErrAddressNotOwnedByUser struct {
	ChainAddress ChainAddress
	UserID       DBID
}

func (e ErrAddressNotOwnedByUser) Error() string {
	return fmt.Sprintf("address is not owned by user: address: %s, userID: %s", e.ChainAddress, e.UserID)
}
