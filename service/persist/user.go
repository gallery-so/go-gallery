package persist

import (
	"context"
	"fmt"
)

// User represents a user in the datase and throughout the application
type User struct {
	Version            NullInt32       `json:"version"` // schema version for this model
	ID                 DBID            `json:"id" binding:"required"`
	CreationTime       CreationTime    `json:"created_at"`
	Deleted            NullBool        `json:"-"`
	LastUpdated        LastUpdatedTime `json:"last_updated"`
	Username           NullString      `json:"username"` // mutable
	UsernameIdempotent NullString      `json:"username_idempotent"`
	Addresses          []Address       `json:"addresses"` // IMPORTANT!! - users can have multiple addresses associated with their account
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
	ExistsByAddress(context.Context, Address) (bool, error)
	Create(context.Context, User) (DBID, error)
	GetByID(context.Context, DBID) (User, error)
	GetByAddress(context.Context, Address) (User, error)
	GetByUsername(context.Context, string) (User, error)
	Delete(context.Context, DBID) error
	AddAddresses(context.Context, DBID, []Address) error
	RemoveAddresses(context.Context, DBID, []Address) error
	MergeUsers(context.Context, DBID, DBID) error
}

// ErrUserNotFound is returned when a user is not found
type ErrUserNotFound struct {
	UserID        DBID
	Address       Address
	Username      string
	Authenticator string
}

func (e ErrUserNotFound) Error() string {
	return fmt.Sprintf("user not found: address: %s, ID: %s, username: %s, authenticator: %s", e.Address, e.UserID, e.Username, e.Authenticator)
}
