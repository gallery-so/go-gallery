package persist

import (
	"context"
	"fmt"
	"time"
)

// User represents a user in the datase and throughout the application
type User struct {
	Version      int64     `bson:"version"` // schema version for this model
	ID           DBID      `bson:"_id"           json:"id" binding:"required"`
	CreationTime time.Time `bson:"created_at" json:"created_at"`
	Deleted      bool      `bson:"deleted" json:"-"`
	LastUpdated  time.Time `bson:"last_updated" json:"last_updated"`

	UserName           string   `bson:"username,omitempty"         json:"username"` // mutable
	UserNameIdempotent string   `bson:"username_idempotent,omitempty" json:"username_idempotent"`
	Addresses          []string `bson:"addresses"     json:"addresses"` // IMPORTANT!! - users can have multiple addresses associated with their account
	Bio                string   `bson:"bio"  json:"bio"`
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
	ExistsByAddress(context.Context, string) (bool, error)
	Create(context.Context, *User) (DBID, error)
	GetByID(context.Context, DBID) (*User, error)
	GetByAddress(context.Context, string) (*User, error)
	GetByUsername(context.Context, string) (*User, error)
	Delete(context.Context, DBID) error
	AddAddresses(context.Context, DBID, []string) error
	RemoveAddresses(context.Context, DBID, []string) error
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
	Address string
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
