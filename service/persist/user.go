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
	Address    Address
	Chain      Chain
	WalletType WalletType
}

// UserRepository represents the interface for interacting with the persisted state of users
type UserRepository interface {
	UpdateByID(context.Context, DBID, interface{}) error
	Create(context.Context, CreateUserInput) (DBID, error)
	AddWallet(context.Context, DBID, Address, Chain, WalletType) error
	RemoveWallet(context.Context, DBID, Address, Chain) error
	GetByID(context.Context, DBID) (User, error)
	GetByWallet(context.Context, DBID) (User, error)
	GetByAddressDetails(context.Context, Address, Chain) (User, error)
	GetByUsername(context.Context, string) (User, error)
	Delete(context.Context, DBID) error
	MergeUsers(context.Context, DBID, DBID) error
	AddFollower(pCtx context.Context, follower DBID, followee DBID) error
	RemoveFollower(pCtx context.Context, follower DBID, followee DBID) error
}

// ErrUserNotFound is returned when a user is not found
type ErrUserNotFound struct {
	UserID        DBID
	WalletID      DBID
	Address       Address
	Chain         Chain
	Username      string
	Authenticator string
}

func (e ErrUserNotFound) Error() string {
	return fmt.Sprintf("user not found: address: %s, ID: %s, walletID: %s,username: %s, authenticator: %s", e.Address, e.WalletID, e.UserID, e.Username, e.Authenticator)
}

type ErrUserAlreadyExists struct {
	Address       Address
	Chain         Chain
	Authenticator string
	Username      string
}

func (e ErrUserAlreadyExists) Error() string {
	return fmt.Sprintf("user already exists: username: %s, address: %s, authenticator: %s", e.Username, e.Address, e.Authenticator)
}
