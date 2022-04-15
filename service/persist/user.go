package persist

import (
	"context"
	"fmt"
)

// UserDB represents a user in the database
type UserDB struct {
	Version            NullInt32       `json:"version"` // schema version for this model
	ID                 DBID            `json:"id" binding:"required"`
	CreationTime       CreationTime    `json:"created_at"`
	Deleted            NullBool        `json:"-"`
	LastUpdated        LastUpdatedTime `json:"last_updated"`
	Username           NullString      `json:"username"` // mutable
	UsernameIdempotent NullString      `json:"username_idempotent"`
	Bio                NullString      `json:"bio"`
}

// User represents a user with all of their addresses
type User struct {
	Version            NullInt32       `json:"version"` // schema version for this model
	ID                 DBID            `json:"id" binding:"required"`
	CreationTime       CreationTime    `json:"created_at"`
	Deleted            NullBool        `json:"-"`
	LastUpdated        LastUpdatedTime `json:"last_updated"`
	Username           NullString      `json:"username"` // mutable
	UsernameIdempotent NullString      `json:"username_idempotent"`
	Addresses          []Wallet        `json:"addresses"` // IMPORTANT!! - users can have multiple addresses associated with their account
	Bio                NullString      `json:"bio"`
}

// UserUpdateInfoInput represents the data to be updated when updating a user
type UserUpdateInfoInput struct {
	LastUpdated        LastUpdatedTime `json:"last_updated"`
	Username           NullString      `json:"username"`
	UsernameIdempotent NullString      `json:"username_idempotent"`
	Bio                NullString      `json:"bio"`
}

// UserRepository represents the interface for interacting with the persisted state of users
type UserRepository interface {
	UpdateByID(context.Context, DBID, interface{}) error
	ExistsByAddress(context.Context, Address, Chain) (bool, error)
	Create(context.Context, User) (DBID, error)
	GetByID(context.Context, DBID) (User, error)
	GetByAddress(context.Context, Address, Chain) (User, error)
	GetByUsername(context.Context, string) (User, error)
	Delete(context.Context, DBID) error
	MergeUsers(context.Context, DBID, DBID) error
}

// ErrUserNotFound is returned when a user is not found
type ErrUserNotFound struct {
	UserID        DBID
	Address       Address
	Chain         Chain
	Username      string
	Authenticator string
}

func (e ErrUserNotFound) Error() string {
	return fmt.Sprintf("user not found: address: %s, ID: %s, username: %s, authenticator: %s", e.Address, e.UserID, e.Username, e.Authenticator)
}
