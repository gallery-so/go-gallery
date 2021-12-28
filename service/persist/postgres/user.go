package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

var insertUsersSQL = "INSERT INTO users (ID, DELETED, VERSION, USERNAME, USERNAME_IDEMPOTENT, ADDRESSES) VALUES ($1, $2, $3, $4, $5, $6)"

// UserRepository represents a user repository in the postgres database
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository creates a new postgres repository for interacting with users
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// UpdateByID updates the user with the given ID
func (u *UserRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUpdate interface{}) error {
	sqlStr := `UPDATE users `
	switch pUpdate.(type) {
	case persist.UserUpdateInfoInput:
		update := pUpdate.(persist.UserUpdateInfoInput)
		sqlStr += `SET USERNAME = $2, USERNAME_IDEMPOTENT = $3, LAST_UPDATED = $4 WHERE ID = $1`
		_, err := u.db.ExecContext(pCtx, sqlStr, pID, update.Username, strings.ToLower(update.UsernameIdempotent), update.LastUpdated)
		if err != nil {
			return err
		}
	default:
		return errors.New("unsupported update type")
	}

	return nil

}

// ExistsByAddress checks if a user exists with the given address
func (u *UserRepository) ExistsByAddress(pCtx context.Context, pAddress persist.Address) (bool, error) {
	sqlStr := `SELECT EXISTS(SELECT 1 FROM users WHERE ADDRESSES @> ARRAY[$1])`

	res, err := u.db.QueryContext(pCtx, sqlStr, pAddress)
	if err != nil {
		return false, err
	}
	defer res.Close()
	var exists bool
	for res.Next() {
		err = res.Scan(&exists)
		if err != nil {
			return false, err
		}
	}

	if err = res.Err(); err != nil {
		return false, err
	}

	return exists, nil
}

// Create creates a new user
func (u *UserRepository) Create(pCtx context.Context, pUser persist.User) (persist.DBID, error) {
	sqlStr := insertUsersSQL + " RETURNING ID"

	var id string
	err := u.db.QueryRowContext(pCtx, sqlStr, persist.GenerateID(), pUser.Deleted, pUser.Version, pUser.Username, pUser.UsernameIdempotent, pq.Array(pUser.Addresses)).Scan(&id)
	if err != nil {
		return "", err
	}

	return persist.DBID(id), nil
}

// GetByID gets the user with the given ID
func (u *UserRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.User, error) {
	sqlStr := `SELECT ID,DELETED,VERSION,USERNANE,USERNAME_IDEMPOTENET,ADDRESSES,CREATED_AT,LAST_UPDATED FROM users WHERE ID = $1`

	res, err := u.db.QueryContext(pCtx, sqlStr, pID)
	if err != nil {
		return persist.User{}, err
	}
	defer res.Close()

	var user persist.User
	for res.Next() {
		err = res.Scan(&user.ID, &user.Deleted, &user.Version, &user.Username, &user.UsernameIdempotent, pq.Array(&user.Addresses), &user.CreationTime, &user.LastUpdated)
		if err != nil {
			return persist.User{}, err
		}
	}

	if err = res.Err(); err != nil {
		return persist.User{}, err
	}

	return user, nil
}

// GetByAddress gets the user with the given address in their list of addresses
func (u *UserRepository) GetByAddress(pCtx context.Context, pAddress persist.Address) (persist.User, error) {
	sqlStr := `SELECT * FROM users WHERE ADDRESSES @> ARRAY[$1]`

	res, err := u.db.QueryContext(pCtx, sqlStr, pAddress)
	if err != nil {
		return persist.User{}, err
	}
	defer res.Close()

	var user persist.User
	for res.Next() {
		err = res.Scan(&user.ID, &user.Deleted, &user.Version, &user.Username, &user.UsernameIdempotent, pq.Array(&user.Addresses), &user.CreationTime, &user.LastUpdated)
		if err != nil {
			return persist.User{}, err
		}
	}

	if err = res.Err(); err != nil {
		return persist.User{}, err
	}

	return user, nil

}

// GetByUsername gets the user with the given username
func (u *UserRepository) GetByUsername(pCtx context.Context, pUsername string) (persist.User, error) {
	sqlStr := `SELECT * FROM users WHERE USERNAME_IDEMPOTENT = $1`

	res, err := u.db.QueryContext(pCtx, sqlStr, strings.ToLower(pUsername))
	if err != nil {
		return persist.User{}, err
	}
	defer res.Close()

	var user persist.User
	for res.Next() {
		err = res.Scan(&user.ID, &user.Deleted, &user.Version, &user.Username, &user.UsernameIdempotent, pq.Array(&user.Addresses), &user.CreationTime, &user.LastUpdated)
		if err != nil {
			return persist.User{}, err
		}
	}

	if err = res.Err(); err != nil {
		return persist.User{}, err
	}

	return user, nil

}

// Delete deletes the user with the given ID
func (u *UserRepository) Delete(pCtx context.Context, pID persist.DBID) error {
	sqlStr := `UPDATE users SET DELETED = TRUE WHERE ID = $1`

	_, err := u.db.ExecContext(pCtx, sqlStr, pID)
	if err != nil {
		return err
	}
	return nil
}

// AddAddresses adds the given addresses to the user with the given ID
func (u *UserRepository) AddAddresses(pCtx context.Context, pID persist.DBID, pAddresses []persist.Address) error {
	sqlStr := `UPDATE users SET ADDRESSES = ADDRESSES || $2 WHERE ID = $1`

	_, err := u.db.ExecContext(pCtx, sqlStr, pID, pAddresses)
	if err != nil {
		return err
	}
	return nil
}

// RemoveAddresses removes the given addresses from the user with the given ID
func (u *UserRepository) RemoveAddresses(pCtx context.Context, pID persist.DBID, pAddresses []persist.Address) error {
	sqlStr := `UPDATE users u SET ADDRESSES = array_remove(u.ADDRESSES, $2) WHERE u.ID = $1`

	_, err := u.db.ExecContext(pCtx, sqlStr, pID, pAddresses)
	if err != nil {
		return err
	}
	return nil
}
