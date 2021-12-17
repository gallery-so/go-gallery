package postgres

import (
	"context"
	"database/sql"
	"strings"

	"github.com/mikeydub/go-gallery/service/persist"
)

var insertUsersSQL = "INSERT INTO users(ID, DELETED, VERSION, USERNAME, USERNAME_IDEMPOTENT, ADDRESSES) VALUES "
var insertUsersValuesSQL = "(?, ?, ?, ?, ?, ?)"

// UserPostgresRepository represents a user repository in the postgres database
type UserPostgresRepository struct {
	db *sql.DB
}

// NewUserPostgresRepository creates a new postgres repository for interacting with users
func NewUserPostgresRepository(db *sql.DB) persist.UserRepository {
	return &UserPostgresRepository{db: db}
}

// UpdateByID updates the user with the given ID
func (u *UserPostgresRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUpdate interface{}) error {
	sqlStr := `UPDATE users `
	sqlStr += prepareSet(pUpdate)
	sqlStr += ` WHERE ID = $1`
	_, err := u.db.ExecContext(pCtx, sqlStr, pID)
	if err != nil {
		return err
	}
	return nil

}

// ExistsByAddress checks if a user exists with the given address
func (u *UserPostgresRepository) ExistsByAddress(pCtx context.Context, pAddress persist.Address) (bool, error) {
	sqlStr := `SELECT EXISTS(SELECT 1 FROM users WHERE ADDRESSES @> ARRAY[$1])`

	res, err := u.db.QueryContext(pCtx, sqlStr, pAddress)
	if err != nil {
		return false, err
	}
	defer res.Close()

	var exists bool
	err = res.Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// Create creates a new user
func (u *UserPostgresRepository) Create(pCtx context.Context, pUser persist.User) (persist.DBID, error) {
	sqlStr := insertUsersSQL + insertUsersValuesSQL + " RETURNING ID"

	res, err := u.db.QueryContext(pCtx, sqlStr, pUser.ID, pUser.Deleted, pUser.Version, pUser.Username, pUser.UsernameIdempotent, pUser.Addresses)
	if err != nil {
		return "", err
	}
	defer res.Close()

	var id string
	err = res.Scan(&id)
	if err != nil {
		return "", err
	}
	return persist.DBID(id), nil
}

// GetByID gets the user with the given ID
func (u *UserPostgresRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.User, error) {
	sqlStr := `SELECT * FROM users WHERE ID = $1`

	res, err := u.db.QueryContext(pCtx, sqlStr, pID)
	if err != nil {
		return persist.User{}, err
	}
	defer res.Close()

	var user persist.User
	for res.Next() {
		err = res.Scan(&user.ID, &user.Deleted, &user.Version, &user.Username, &user.UsernameIdempotent, &user.Addresses, &user.CreationTime, &user.LastUpdated)
		if err != nil {
			return persist.User{}, err
		}
	}
	return user, nil
}

// GetByAddress gets the user with the given address in their list of addresses
func (u *UserPostgresRepository) GetByAddress(pCtx context.Context, pAddress persist.Address) (persist.User, error) {
	sqlStr := `SELECT * FROM users WHERE ADDRESSES @> ARRAY[$1]`

	res, err := u.db.QueryContext(pCtx, sqlStr, pAddress)
	if err != nil {
		return persist.User{}, err
	}
	defer res.Close()

	var user persist.User
	for res.Next() {
		err = res.Scan(&user.ID, &user.Deleted, &user.Version, &user.Username, &user.UsernameIdempotent, &user.Addresses, &user.CreationTime, &user.LastUpdated)
		if err != nil {
			return persist.User{}, err
		}
	}
	return user, nil

}

// GetByUsername gets the user with the given username
func (u *UserPostgresRepository) GetByUsername(pCtx context.Context, pUsername string) (persist.User, error) {
	sqlStr := `SELECT * FROM users WHERE USERNAME_IDEMPOTENT = $1`

	res, err := u.db.QueryContext(pCtx, sqlStr, strings.ToLower(pUsername))
	if err != nil {
		return persist.User{}, err
	}
	defer res.Close()

	var user persist.User
	for res.Next() {
		err = res.Scan(&user.ID, &user.Deleted, &user.Version, &user.Username, &user.UsernameIdempotent, &user.Addresses, &user.CreationTime, &user.LastUpdated)
		if err != nil {
			return persist.User{}, err
		}
	}
	return user, nil

}

// Delete deletes the user with the given ID
func (u *UserPostgresRepository) Delete(pCtx context.Context, pID persist.DBID) error {
	sqlStr := `UPDATE users SET DELETED = TRUE WHERE ID = $1`

	_, err := u.db.ExecContext(pCtx, sqlStr, pID)
	if err != nil {
		return err
	}
	return nil
}

// AddAddresses adds the given addresses to the user with the given ID
func (u *UserPostgresRepository) AddAddresses(pCtx context.Context, pID persist.DBID, pAddresses []persist.Address) error {
	sqlStr := `UPDATE users SET ADDRESSES = ADDRESSES || $2 WHERE ID = $1`

	_, err := u.db.ExecContext(pCtx, sqlStr, pID, pAddresses)
	if err != nil {
		return err
	}
	return nil
}

// RemoveAddresses removes the given addresses from the user with the given ID
func (u *UserPostgresRepository) RemoveAddresses(pCtx context.Context, pID persist.DBID, pAddresses []persist.Address) error {
	sqlStr := `UPDATE users SET ADDRESSES = array_diff(ADDRESSES, $2) WHERE ID = $1`

	_, err := u.db.ExecContext(pCtx, sqlStr, pID, pAddresses)
	if err != nil {
		return err
	}
	return nil
}
