package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/lib/pq"
)

type Traits map[string]interface{}

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
	Traits             Traits          `json:"traits"`
	Universal          NullBool        `json:"universal"`
}

// UserUpdateInfoInput represents the data to be updated when updating a user
type UserUpdateInfoInput struct {
	LastUpdated        LastUpdatedTime `json:"last_updated"`
	Username           NullString      `json:"username"`
	UsernameIdempotent NullString      `json:"username_idempotent"`
	Bio                NullString      `json:"bio"`
}

// UserUpdateNotificationSettings represents the data to be updated when updating a user's notification settings
type UserUpdateNotificationSettings struct {
	LastUpdated          LastUpdatedTime          `json:"last_updated"`
	NotificationSettings UserNotificationSettings `json:"notification_settings"`
}

/*
 	someoneFollowedYou: Boolean
    someoneAdmiredYourUpdate: Boolean
    someoneCommentedOnYourUpdate: Boolean
    someoneViewedYourGallery: Boolean
*/
type UserNotificationSettings struct {
	SomeoneFollowedYou           *bool `json:"someone_followed_you,omitempty"`
	SomeoneAdmiredYourUpdate     *bool `json:"someone_admired_your_update,omitempty"`
	SomeoneCommentedOnYourUpdate *bool `json:"someone_commented_on_your_update,omitempty"`
	SomeoneViewedYourGallery     *bool `json:"someone_viewed_your_gallery,omitempty"`
}

type CreateUserInput struct {
	Username                   string
	Bio                        string
	Email                      *string
	ChainAddress               ChainAddress
	WalletType                 WalletType
	Universal                  bool
	EmailNotificationsSettings EmailUnsubscriptions
}

// UserRepository represents the interface for interacting with the persisted state of users
type UserRepository interface {
	UpdateByID(context.Context, DBID, interface{}) error
	Create(context.Context, CreateUserInput) (DBID, error)
	AddWallet(context.Context, DBID, ChainAddress, WalletType) error
	RemoveWallet(context.Context, DBID, DBID) error
	GetByID(context.Context, DBID) (User, error)
	GetByIDs(context.Context, []DBID) ([]User, error)
	GetByWalletID(context.Context, DBID) (User, error)
	GetByChainAddress(context.Context, ChainAddress) (User, error)
	GetByUsername(context.Context, string) (User, error)
	Delete(context.Context, DBID) error
	MergeUsers(context.Context, DBID, DBID) error
	AddFollower(pCtx context.Context, follower DBID, followee DBID) (refollowed bool, err error)
	RemoveFollower(pCtx context.Context, follower DBID, followee DBID) error
	UserFollowsUser(pCtx context.Context, userA DBID, userB DBID) (bool, error)
	FillWalletDataForUser(pCtx context.Context, user *User) error
}

// Scan implements the database/sql Scanner interface for the Traits type
func (m *Traits) Scan(src interface{}) error {
	if src == nil {
		*m = Traits{}
		return nil
	}
	return json.Unmarshal(src.([]uint8), m)
}

// Value implements the database/sql/driver Valuer interface for the Traits type
func (m Traits) Value() (driver.Value, error) {
	val, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	return []byte(strings.ToValidUTF8(strings.ReplaceAll(string(val), "\\u0000", ""), "")), nil
}

func (u UserNotificationSettings) Value() (driver.Value, error) {
	return json.Marshal(u)
}

func (u *UserNotificationSettings) Scan(src interface{}) error {
	if src == nil {
		*u = UserNotificationSettings{}
		return nil
	}
	return json.Unmarshal(src.([]uint8), u)
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
	template := "user not found: %s;authMethod=%s"

	if e.UserID != "" {
		method := fmt.Sprintf("method=%s;userID=%s", "byUserID", e.UserID)
		return fmt.Sprintf(template, method, e.Authenticator)
	}

	if e.WalletID != "" {
		method := fmt.Sprintf("method=%s;walletID=%s", "byWalletID", e.WalletID)
		return fmt.Sprintf(template, method, e.Authenticator)
	}

	if e.Username != "" {
		method := fmt.Sprintf("method=%s;username=%s", "byUsername", e.Username)
		return fmt.Sprintf(template, method, e.Authenticator)
	}

	if e.ChainAddress != (ChainAddress{}) {
		method := fmt.Sprintf("method=%s;chainAddress=%s", "byChainAddress", e.ChainAddress)
		return fmt.Sprintf(template, method, e.Authenticator)
	}

	return fmt.Sprintf("user not found:authMethod=%s", e.Authenticator)
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

type ErrWalletCreateFailed struct {
	ChainAddress ChainAddress
	WalletID     DBID
	Err          error
}

func (e ErrWalletCreateFailed) Error() string {
	return fmt.Sprintf("wallet create failed: address: %s, walletID: %s, error: %s", e.ChainAddress, e.WalletID, e.Err)
}

type Role string

const (
	RoleAdmin      Role = "ADMIN"
	RoleBetaTester      = "BETA_TESTER"
)

// Scan implements the database/sql Scanner interface for the DBID type
func (r *Role) Scan(i interface{}) error {
	if i == nil {
		return nil
	}
	if it, ok := i.([]uint8); ok {
		*r = Role(it)
		return nil
	}
	*r = Role(i.(string))
	return nil
}

// Value implements the database/sql driver Valuer interface for the DBID type
func (r *Role) Value() (driver.Value, error) {
	return r, nil
}

// UnmarshalGQL implements the graphql.Unmarshaler interface
func (r *Role) UnmarshalGQL(v interface{}) error {
	n, ok := v.(string)
	if !ok {
		return fmt.Errorf("Role must be a string")
	}

	switch strings.ToLower(n) {
	case "admin":
		*r = RoleAdmin
	case "beta_tester":
		*r = RoleBetaTester
	}
	return nil
}

// MarshalGQL implements the graphql.Marshaler interface
func (r Role) MarshalGQL(w io.Writer) {
	switch r {
	case RoleAdmin:
		w.Write([]byte(`"ADMIN"`))
	case RoleBetaTester:
		w.Write([]byte(`"BETA_TESTER"`))
	}
}

type RoleList []Role

func (l RoleList) Value() (driver.Value, error) {
	return pq.Array(l).Value()
}

func (l *RoleList) Scan(value interface{}) error {
	return pq.Array(l).Scan(value)
}
