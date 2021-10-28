package persist

import (
	"context"
	"fmt"
)

// User represents a user in the datase and throughout the application
type User struct {
	Version      int64           `bson:"version"` // schema version for this model
	ID           DBID            `bson:"_id,id"           json:"id" binding:"required"`
	CreationTime CreationTime    `bson:"created_at,creation_time" json:"created_at"`
	Deleted      bool            `bson:"deleted" json:"-"`
	LastUpdated  LastUpdatedTime `bson:"last_updated,update_time" json:"last_updated"`

	UserName           string    `bson:"username,omitempty"         json:"username"` // mutable
	UserNameIdempotent string    `bson:"username_idempotent,omitempty" json:"username_idempotent"`
	Addresses          []Address `bson:"addresses"     json:"addresses"` // IMPORTANT!! - users can have multiple addresses associated with their account
	Bio                string    `bson:"bio"  json:"bio"`
}

// UserUpdateInfoInput represents the data to be updated when updating a user
type UserUpdateInfoInput struct {
	UserName           string `bson:"username"`
	UserNameIdempotent string `bson:"username_idempotent"`
	Bio                string `bson:"bio"`
}

// UserRepository represents the interface for interacting with the persisted state of users
type UserRepository interface {
	UpdateByID(context.Context, DBID, interface{}) error
	ExistsByAddress(context.Context, Address) (bool, error)
	Create(context.Context, *User) (DBID, error)
	GetByID(context.Context, DBID) (*User, error)
	GetByAddress(context.Context, Address) (*User, error)
	GetByUsername(context.Context, string) (*User, error)
	Delete(context.Context, DBID) error
	AddAddresses(context.Context, DBID, []Address) error
	RemoveAddresses(context.Context, DBID, []Address) error
}

// ErrUserNotFoundByID is returned when a user is not found by ID
type ErrUserNotFoundByID struct {
	ID DBID
}

// ErrUserNotFoundByUsername is returned when a user is not found by username
type ErrUserNotFoundByUsername struct {
	Username string
}

// ErrUserNotFoundByAddress is returned when a user is not found by wallet address
type ErrUserNotFoundByAddress struct {
	Address Address
}

func (e ErrUserNotFoundByID) Error() string {
	return fmt.Sprintf("user not found by ID: %s", e.ID)
}

func (e ErrUserNotFoundByAddress) Error() string {
	return fmt.Sprintf("user not found by address: %s", e.Address)
}

func (e ErrUserNotFoundByUsername) Error() string {
	return fmt.Sprintf("user not found by username: %s", e.Username)
}
