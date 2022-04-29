package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

// UserRepository represents a user repository in the postgres database
type UserRepository struct {
	db                    *sql.DB
	updateInfoStmt        *sql.Stmt
	existsByAddressStmt   *sql.Stmt
	createStmt            *sql.Stmt
	getByIDStmt           *sql.Stmt
	getByAddressStmt      *sql.Stmt
	getByUsernameStmt     *sql.Stmt
	deleteStmt            *sql.Stmt
	getGalleriesStmt      *sql.Stmt
	updateCollectionsStmt *sql.Stmt
	deleteGalleryStmt     *sql.Stmt
	createAddressStmt     *sql.Stmt
	getAddressStmt        *sql.Stmt
	createWalletStmt      *sql.Stmt
	getWalletStmt         *sql.Stmt
	addWalletStmt         *sql.Stmt
	removeWalletStmt      *sql.Stmt
}

// NewUserRepository creates a new postgres repository for interacting with users
// TODO joins for users to wallets and wallets to addresses
func NewUserRepository(db *sql.DB) *UserRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	updateInfoStmt, err := db.PrepareContext(ctx, `UPDATE users SET USERNAME = $2, USERNAME_IDEMPOTENT = $3, LAST_UPDATED = $4, BIO = $5 WHERE ID = $1;`)
	checkNoErr(err)

	existsByAddressStmt, err := db.PrepareContext(ctx, `SELECT 1 FROM users WHERE EXISTS(SELECT 1 FROM wallets WHERE ADDRESS = $1 AND CHAIN = $2 AND USER_ID = users.ID AND DELETED = false);`)
	checkNoErr(err)

	createStmt, err := db.PrepareContext(ctx, `INSERT INTO users (ID, DELETED, VERSION, USERNAME, USERNAME_IDEMPOTENT, ADDRESSES) VALUES ($1, $2, $3, $4, $5, $6) RETURNING ID;`)
	checkNoErr(err)

	getByIDStmt, err := db.PrepareContext(ctx, `SELECT ID,DELETED,VERSION,USERNAME,USERNAME_IDEMPOTENT,ADDRESSES,BIO,CREATED_AT,LAST_UPDATED FROM users WHERE ID = $1 AND DELETED = false;`)
	checkNoErr(err)

	getByAddressStmt, err := db.PrepareContext(ctx, `SELECT ID,DELETED,VERSION,USERNAME,USERNAME_IDEMPOTENT,ADDRESSES,BIO,CREATED_AT,LAST_UPDATED FROM users WHERE ID = (SELECT USER_ID FROM wallets WHERE ADDRESS = $1 AND CHAIN = $2 AND DELETED = false) AND DELETED = false;`)
	checkNoErr(err)

	getByUsernameStmt, err := db.PrepareContext(ctx, `SELECT ID,DELETED,VERSION,USERNAME,USERNAME_IDEMPOTENT,ADDRESSES,BIO,CREATED_AT,LAST_UPDATED FROM users WHERE USERNAME_IDEMPOTENT = $1 AND DELETED = false;`)
	checkNoErr(err)

	deleteStmt, err := db.PrepareContext(ctx, `UPDATE users SET DELETED = TRUE WHERE ID = $1;`)
	checkNoErr(err)

	getGalleriesStmt, err := db.PrepareContext(ctx, `SELECT ID, COLLECTIONS FROM galleries WHERE OWNER_USER_ID = $1 and DELETED = false;`)
	checkNoErr(err)

	updateCollectionsStmt, err := db.PrepareContext(ctx, `UPDATE galleries SET COLLECTIONS = $2 WHERE ID = $1;`)
	checkNoErr(err)

	deleteGalleryStmt, err := db.PrepareContext(ctx, `UPDATE galleries SET DELETED = true WHERE ID = $1;`)
	checkNoErr(err)

	createAddressStmt, err := db.PrepareContext(ctx, `INSERT INTO addresses (ID, ADDRESS, CHAIN) VALUES ($1, $2, $3) ON CONFLICT (ADDRESS,CHAIN) DO NOTHING;`)
	checkNoErr(err)

	getAddressStmt, err := db.PrepareContext(ctx, `SELECT ID, ADDRESS, CHAIN FROM addresses WHERE ADDRESS = $1 AND CHAIN = $2;`)
	checkNoErr(err)

	createWalletStmt, err := db.PrepareContext(ctx, `INSERT INTO wallets (ID, ADDRESS, TYPE) VALUES ($1, $2, $3) ON CONFLICT (ADDRESS) DO NOTHING;`)
	checkNoErr(err)

	getWalletStmt, err := db.PrepareContext(ctx, `SELECT ID, ADDRESS FROM wallets WHERE ADDRESS = $1;`)
	checkNoErr(err)

	addWalletStmt, err := db.PrepareContext(ctx, `UPDATE users SET ADDRESSES = array_append(ADDRESSES, $1) WHERE ID = $2;`)
	checkNoErr(err)

	removeWalletStmt, err := db.PrepareContext(ctx, `UPDATE users SET ADDRESSES = array_remove(ADDRESSES, $1) WHERE ID = $2;`)
	checkNoErr(err)

	return &UserRepository{
		db:                  db,
		updateInfoStmt:      updateInfoStmt,
		existsByAddressStmt: existsByAddressStmt,
		createStmt:          createStmt,
		getByIDStmt:         getByIDStmt,
		getByAddressStmt:    getByAddressStmt,
		getByUsernameStmt:   getByUsernameStmt,
		deleteStmt:          deleteStmt,

		getGalleriesStmt:      getGalleriesStmt,
		updateCollectionsStmt: updateCollectionsStmt,
		deleteGalleryStmt:     deleteGalleryStmt,
		createAddressStmt:     createAddressStmt,
		getAddressStmt:        getAddressStmt,
		createWalletStmt:      createWalletStmt,
		getWalletStmt:         getWalletStmt,
		addWalletStmt:         addWalletStmt,
		removeWalletStmt:      removeWalletStmt,
	}
}

// UpdateByID updates the user with the given ID
func (u *UserRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUpdate interface{}) error {

	switch pUpdate.(type) {
	case persist.UserUpdateInfoInput:
		update := pUpdate.(persist.UserUpdateInfoInput)
		aUser, _ := u.GetByUsername(pCtx, update.Username.String())
		if aUser.ID != "" && aUser.ID != pID {
			return fmt.Errorf("username %s already exists", update.Username.String())
		}
		res, err := u.updateInfoStmt.ExecContext(pCtx, pID, update.Username, strings.ToLower(update.UsernameIdempotent.String()), update.LastUpdated, update.Bio)
		if err != nil {
			return err
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return persist.ErrUserNotFound{UserID: pID}
		}
	default:
		return fmt.Errorf("unsupported update type: %T", pUpdate)
	}

	return nil

}

// ExistsByAddress checks if a user exists with the given address
// TODO use string and chain to get the user
func (u *UserRepository) ExistsByAddress(pCtx context.Context, pAddress persist.AddressValue, pChain persist.Chain) (bool, error) {

	res, err := u.existsByAddressStmt.QueryContext(pCtx, pAddress, pChain)
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
func (u *UserRepository) Create(pCtx context.Context, pUser persist.CreateUserInput) (persist.DBID, error) {
	// TODO handle creating wallet
	var id persist.DBID
	err := u.createStmt.QueryRowContext(pCtx, persist.GenerateID(), false, 0).Scan(&id)
	if err != nil {
		return "", err
	}

	return id, nil
}

// GetByID gets the user with the given ID
func (u *UserRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.User, error) {

	user := persist.User{}
	err := u.getByIDStmt.QueryRowContext(pCtx, pID).Scan(&user.ID, &user.Deleted, &user.Version, &user.Username, &user.UsernameIdempotent, pq.Array(&user.Wallets), &user.Bio, &user.CreationTime, &user.LastUpdated)
	if err != nil {
		if err == sql.ErrNoRows {
			return persist.User{}, persist.ErrUserNotFound{UserID: pID}
		}
		return persist.User{}, err
	}
	return user, nil
}

// GetByAddress gets the user with the given address in their list of addresses
// TODO use string and chain to get the user
func (u *UserRepository) GetByAddress(pCtx context.Context, pAddress persist.AddressValue, pChain persist.Chain) (persist.User, error) {

	var user persist.User
	err := u.getByAddressStmt.QueryRowContext(pCtx, pAddress).Scan(&user.ID, &user.Deleted, &user.Version, &user.Username, &user.UsernameIdempotent, pq.Array(&user.Wallets), &user.Bio, &user.CreationTime, &user.LastUpdated)
	if err != nil {
		if err == sql.ErrNoRows {
			return persist.User{}, persist.ErrUserNotFound{Address: pAddress, Chain: pChain}
		}
		return persist.User{}, err
	}

	return user, nil

}

// GetByUsername gets the user with the given username
func (u *UserRepository) GetByUsername(pCtx context.Context, pUsername string) (persist.User, error) {

	var user persist.User
	err := u.getByUsernameStmt.QueryRowContext(pCtx, strings.ToLower(pUsername)).Scan(&user.ID, &user.Deleted, &user.Version, &user.Username, &user.UsernameIdempotent, pq.Array(&user.Wallets), &user.Bio, &user.CreationTime, &user.LastUpdated)
	if err != nil {
		if err == sql.ErrNoRows {
			return persist.User{}, persist.ErrUserNotFound{Username: pUsername}
		}
		return persist.User{}, err
	}
	return user, nil

}

// AddWallet adds an address to user as well as ensures that the wallet and address exists
func (u *UserRepository) AddWallet(pCtx context.Context, pUserID persist.DBID, pAddress persist.AddressValue, pChain persist.Chain, pWalletType persist.WalletType) error {
	if _, err := u.createAddressStmt.ExecContext(pCtx, pAddress, pChain); err != nil {
		return err
	}
	var addrID persist.DBID
	err := u.getAddressStmt.QueryRowContext(pCtx, pAddress, pChain).Scan(&addrID)
	if err != nil {
		return err
	}

	if _, err := u.createWalletStmt.ExecContext(pCtx, addrID, pWalletType); err != nil {
		return err
	}

	var walletID persist.DBID
	err = u.getWalletStmt.QueryRowContext(pCtx, addrID).Scan(&walletID)
	if err != nil {
		return err
	}

	if _, err := u.addWalletStmt.ExecContext(pCtx, walletID, pUserID); err != nil {
		return err
	}

	return nil
}

// RemoveWallet removes an address from user
func (u *UserRepository) RemoveWallet(pCtx context.Context, pUserID persist.DBID, pAddress persist.AddressValue, pChain persist.Chain) error {
	var addrID persist.DBID
	err := u.getAddressStmt.QueryRowContext(pCtx, pAddress, pChain).Scan(&addrID)
	if err != nil {
		return err
	}

	var walletID persist.DBID
	err = u.getWalletStmt.QueryRowContext(pCtx, addrID).Scan(&walletID)
	if err != nil {
		return err
	}

	if _, err := u.removeWalletStmt.ExecContext(pCtx, walletID, pUserID); err != nil {
		return err
	}

	return nil
}

// Delete deletes the user with the given ID
func (u *UserRepository) Delete(pCtx context.Context, pID persist.DBID) error {

	res, err := u.deleteStmt.ExecContext(pCtx, pID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return persist.ErrUserNotFound{UserID: pID}
	}

	return nil
}

// MergeUsers merges the given users into the first user
func (u *UserRepository) MergeUsers(pCtx context.Context, pInitialUser persist.DBID, pSecondUser persist.DBID) error {

	var user persist.User
	if err := u.getByIDStmt.QueryRowContext(pCtx, pInitialUser).Scan(&user.ID, &user.Deleted, &user.Version, &user.Username, &user.UsernameIdempotent, pq.Array(&user.Wallets), &user.Bio, &user.CreationTime, &user.LastUpdated); err != nil {
		return err
	}

	var secondUser persist.User
	if err := u.getByIDStmt.QueryRowContext(pCtx, pSecondUser).Scan(&secondUser.ID, &secondUser.Deleted, &secondUser.Version, &secondUser.Username, &secondUser.UsernameIdempotent, pq.Array(&secondUser.Wallets), &secondUser.Bio, &secondUser.CreationTime, &secondUser.LastUpdated); err != nil {
		return err
	}

	tx, err := u.db.BeginTx(pCtx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	deleteGalleryStmt := tx.StmtContext(pCtx, u.deleteGalleryStmt)
	mergedCollections := make([]persist.DBID, 0, 3)

	res, err := u.getGalleriesStmt.QueryContext(pCtx, secondUser.ID)
	if err != nil {
		return err
	}
	defer res.Close()

	for res.Next() {
		var gallery persist.GalleryDB
		if err := res.Scan(&gallery.ID, pq.Array(&gallery.Collections)); err != nil {
			return err
		}

		mergedCollections = append(mergedCollections, gallery.Collections...)

		if _, err := deleteGalleryStmt.ExecContext(pCtx, gallery.ID); err != nil {
			return err
		}
	}

	if err := res.Err(); err != nil {
		return err
	}

	nextRes, err := u.getGalleriesStmt.QueryContext(pCtx, pInitialUser)
	if err != nil {
		return err
	}
	defer nextRes.Close()

	if nextRes.Next() {
		var gallery persist.GalleryDB
		if err := nextRes.Scan(&gallery.ID, pq.Array(&gallery.Collections)); err != nil {
			return err
		}

		if _, err := tx.StmtContext(pCtx, u.updateCollectionsStmt).ExecContext(pCtx, gallery.ID, pq.Array(append(gallery.Collections, mergedCollections...))); err != nil {
			return err
		}
	}

	if _, err := tx.StmtContext(pCtx, u.deleteStmt).ExecContext(pCtx, secondUser.ID); err != nil {
		return err
	}

	return tx.Commit()
}
