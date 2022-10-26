package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
)

// UserRepository represents a user repository in the postgres database
type UserRepository struct {
	db                       *sql.DB
	updateInfoStmt           *sql.Stmt
	createStmt               *sql.Stmt
	getByIDStmt              *sql.Stmt
	getByIDsStmt             *sql.Stmt
	getByWalletIDStmt        *sql.Stmt
	getByUsernameStmt        *sql.Stmt
	deleteStmt               *sql.Stmt
	getGalleriesStmt         *sql.Stmt
	updateCollectionsStmt    *sql.Stmt
	deleteGalleryStmt        *sql.Stmt
	createWalletStmt         *sql.Stmt
	getWalletIDStmt          *sql.Stmt
	getWalletStmt            *sql.Stmt
	addWalletStmt            *sql.Stmt
	removeWalletFromUserStmt *sql.Stmt
	deleteWalletStmt         *sql.Stmt
	addFollowerStmt          *sql.Stmt
	removeFollowerStmt       *sql.Stmt
	followsUserStmt          *sql.Stmt
	userHasTrait             *sql.Stmt

	queries *coredb.Queries
}

// NewUserRepository creates a new postgres repository for interacting with users
// TODO joins for users to wallets and wallets to addresses
func NewUserRepository(db *sql.DB) *UserRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	updateInfoStmt, err := db.PrepareContext(ctx, `UPDATE users SET USERNAME = $2, USERNAME_IDEMPOTENT = $3, LAST_UPDATED = $4, BIO = $5 WHERE ID = $1;`)
	checkNoErr(err)

	createStmt, err := db.PrepareContext(ctx, `INSERT INTO users (ID, USERNAME, USERNAME_IDEMPOTENT, BIO, WALLETS, UNIVERSAL) VALUES ($1, $2, $3, $4, $5, $6) RETURNING ID;`)
	checkNoErr(err)

	getByIDStmt, err := db.PrepareContext(ctx, `SELECT ID,DELETED,VERSION,USERNAME,USERNAME_IDEMPOTENT,BIO,TRAITS,WALLETS,UNIVERSAL,CREATED_AT,LAST_UPDATED FROM users WHERE ID = $1 AND DELETED = false;`)
	checkNoErr(err)

	getByIDsStmt, err := db.PrepareContext(ctx, `SELECT ID,DELETED,VERSION,USERNAME,USERNAME_IDEMPOTENT,BIO,TRAITS,WALLETS,UNIVERSAL,CREATED_AT,LAST_UPDATED FROM users WHERE ID = ANY($1) AND DELETED = false;`)
	checkNoErr(err)

	getByWalletIDStmt, err := db.PrepareContext(ctx, `SELECT ID,DELETED,VERSION,USERNAME,USERNAME_IDEMPOTENT,WALLETS,BIO,TRAITS,UNIVERSAL,CREATED_AT,LAST_UPDATED FROM users WHERE ARRAY[$1]::varchar[] <@ WALLETS AND DELETED = false;`)
	checkNoErr(err)

	getByUsernameStmt, err := db.PrepareContext(ctx, `SELECT ID,DELETED,VERSION,USERNAME,USERNAME_IDEMPOTENT,WALLETS,BIO,TRAITS,UNIVERSAL,CREATED_AT,LAST_UPDATED FROM users WHERE USERNAME_IDEMPOTENT = $1 AND DELETED = false AND UNIVERSAL = false;`)
	checkNoErr(err)

	deleteStmt, err := db.PrepareContext(ctx, `UPDATE users SET DELETED = TRUE WHERE ID = $1;`)
	checkNoErr(err)

	getGalleriesStmt, err := db.PrepareContext(ctx, `SELECT ID, COLLECTIONS FROM galleries WHERE OWNER_USER_ID = $1 and DELETED = false;`)
	checkNoErr(err)

	updateCollectionsStmt, err := db.PrepareContext(ctx, `UPDATE galleries SET COLLECTIONS = $2 WHERE ID = $1;`)
	checkNoErr(err)

	deleteGalleryStmt, err := db.PrepareContext(ctx, `UPDATE galleries SET DELETED = true WHERE ID = $1;`)
	checkNoErr(err)

	createWalletStmt, err := db.PrepareContext(ctx, `INSERT INTO wallets (ID, ADDRESS, CHAIN, WALLET_TYPE) VALUES ($1, $2, $3, $4);`)
	checkNoErr(err)

	getWalletIDStmt, err := db.PrepareContext(ctx, `SELECT ID FROM wallets WHERE ADDRESS = $1 AND CHAIN = $2 AND DELETED = false;`)
	checkNoErr(err)

	getWalletStmt, err := db.PrepareContext(ctx, `SELECT ADDRESS,CHAIN,WALLET_TYPE,VERSION,CREATED_AT,LAST_UPDATED FROM wallets WHERE ID = $1 AND DELETED = false;`)
	checkNoErr(err)

	addWalletStmt, err := db.PrepareContext(ctx, `UPDATE users SET WALLETS = array_append(WALLETS, $1) WHERE ID = $2;`)
	checkNoErr(err)

	removeWalletFromUserStmt, err := db.PrepareContext(ctx, `UPDATE users SET WALLETS = array_remove(WALLETS, $1) WHERE ID = $2;`)
	checkNoErr(err)

	deleteWalletStmt, err := db.PrepareContext(ctx, `UPDATE wallets SET DELETED = true, LAST_UPDATED = NOW() WHERE ID = $1;`)
	checkNoErr(err)

	addFollowerStmt, err := db.PrepareContext(ctx, `INSERT INTO follows (ID, FOLLOWER, FOLLOWEE, DELETED) VALUES ($1, $2, $3, false) ON CONFLICT (FOLLOWER, FOLLOWEE) DO UPDATE SET deleted = false, LAST_UPDATED = now() RETURNING LAST_UPDATED > CREATED_AT;`)
	checkNoErr(err)

	removeFollowerStmt, err := db.PrepareContext(ctx, `UPDATE follows SET DELETED = true, LAST_UPDATED = NOW() WHERE FOLLOWER = $1 AND FOLLOWEE = $2`)
	checkNoErr(err)

	followsUserStmt, err := db.PrepareContext(ctx, `SELECT EXISTS(SELECT 1 FROM follows WHERE follower = $1 AND followee = $2 AND deleted = false);`)
	checkNoErr(err)

	return &UserRepository{
		db:                db,
		updateInfoStmt:    updateInfoStmt,
		createStmt:        createStmt,
		getByIDStmt:       getByIDStmt,
		getByIDsStmt:      getByIDsStmt,
		getByWalletIDStmt: getByWalletIDStmt,
		getByUsernameStmt: getByUsernameStmt,
		deleteStmt:        deleteStmt,

		getGalleriesStmt:         getGalleriesStmt,
		updateCollectionsStmt:    updateCollectionsStmt,
		deleteGalleryStmt:        deleteGalleryStmt,
		createWalletStmt:         createWalletStmt,
		getWalletIDStmt:          getWalletIDStmt,
		getWalletStmt:            getWalletStmt,
		addWalletStmt:            addWalletStmt,
		removeWalletFromUserStmt: removeWalletFromUserStmt,
		deleteWalletStmt:         deleteWalletStmt,
		addFollowerStmt:          addFollowerStmt,
		removeFollowerStmt:       removeFollowerStmt,
		followsUserStmt:          followsUserStmt,
	}
}

// UpdateByID updates the user with the given ID
func (u *UserRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUpdate interface{}) error {

	switch pUpdate.(type) {
	case persist.UserUpdateInfoInput:
		update := pUpdate.(persist.UserUpdateInfoInput)
		aUser, err := u.GetByUsername(pCtx, update.Username.String())
		if err != nil {
			errNotFound := persist.ErrUserNotFound{}
			if !errors.As(err, &errNotFound) {
				return err
			}
		} else {
			if aUser.ID != "" && aUser.ID != pID {
				return persist.ErrUsernameNotAvailable{Username: update.Username.String()}
			}
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
	case persist.UserUpdateNotificationSettings:
		update := pUpdate.(persist.UserUpdateNotificationSettings)
		return u.queries.UpdateNotificationSettingsByID(pCtx, coredb.UpdateNotificationSettingsByIDParams{
			ID:                   pID,
			NotificationSettings: update.NotificationSettings,
		})
	default:
		return fmt.Errorf("unsupported update type: %T", pUpdate)
	}

	return nil

}

func (u *UserRepository) createWalletWithTx(ctx context.Context, tx *sql.Tx, chainAddress persist.ChainAddress, walletType persist.WalletType) (persist.DBID, error) {
	// Do we already have a wallet with this ChainAddress?
	var walletID persist.DBID
	if err := tx.StmtContext(ctx, u.getWalletIDStmt).QueryRowContext(ctx, chainAddress.Address(), chainAddress.Chain()).Scan(&walletID); err != nil && err != sql.ErrNoRows {
		return "", err
	}

	if walletID != "" {
		// If we do have a wallet with this ChainAddress, does it belong to a user?
		var user persist.User
		err := tx.StmtContext(ctx, u.getByWalletIDStmt).QueryRowContext(ctx, walletID).Scan(&user.ID, &user.Deleted, &user.Version, &user.Username, &user.UsernameIdempotent, pq.Array(&user.Wallets), &user.Bio, &user.Traits, &user.Universal, &user.CreationTime, &user.LastUpdated)
		if err != nil && err != sql.ErrNoRows {
			return "", err
		}

		// If the wallet belongs to a user, its address can't be used to create a new wallet. Return an error.
		if user.ID != "" {
			return "", persist.ErrAddressOwnedByUser{ChainAddress: chainAddress, OwnerID: user.ID}
		}

		logger.For(ctx).Infof("wallet %s already exists, but is not owned by a user", walletID)
		// If the wallet exists but doesn't belong to anyone, it should be deleted.
		if _, err := tx.StmtContext(ctx, u.deleteWalletStmt).ExecContext(ctx, walletID); err != nil {
			return "", err
		}
	}

	newWalletID := persist.GenerateID()
	// At this point, we know there's no existing wallet in the database with this ChainAddress, so let's make a new one!
	if _, err := tx.StmtContext(ctx, u.createWalletStmt).ExecContext(ctx, newWalletID, chainAddress.Address(), chainAddress.Chain(), walletType); err != nil {
		return "", persist.ErrWalletCreateFailed{
			ChainAddress: chainAddress,
			WalletID:     walletID,
			Err:          err,
		}
	}

	return newWalletID, nil
}

// Create creates a new user
func (u *UserRepository) Create(pCtx context.Context, pUser persist.CreateUserInput) (persist.DBID, error) {
	tx, err := u.db.BeginTx(pCtx, nil)
	if err != nil {
		return "", err
	}

	defer tx.Rollback()

	var user persist.User
	err = tx.StmtContext(pCtx, u.getByUsernameStmt).QueryRowContext(pCtx, strings.ToLower(pUser.Username)).Scan(&user.ID, &user.Deleted, &user.Version, &user.Username, &user.UsernameIdempotent, pq.Array(&user.Wallets), &user.Bio, &user.Traits, &user.Universal, &user.CreationTime, &user.LastUpdated)
	if err == nil {
		return "", persist.ErrUsernameNotAvailable{Username: pUser.Username}
	}

	// If there are no rows returned, the username isn't taken yet
	if err != sql.ErrNoRows {
		return "", err
	}

	walletID, err := u.createWalletWithTx(pCtx, tx, pUser.ChainAddress, pUser.WalletType)
	if err != nil {
		return "", err
	}

	var id persist.DBID
	err = tx.StmtContext(pCtx, u.createStmt).QueryRowContext(pCtx, persist.GenerateID(), pUser.Username, strings.ToLower(pUser.Username), pUser.Bio, []persist.DBID{walletID}, pUser.Universal).Scan(&id)
	if err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	return id, nil
}

// GetByID gets the user with the given ID
func (u *UserRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.User, error) {

	user := persist.User{}
	walletIDs := []persist.DBID{}
	err := u.getByIDStmt.QueryRowContext(pCtx, pID).Scan(&user.ID, &user.Deleted, &user.Version, &user.Username, &user.UsernameIdempotent, &user.Bio, &user.Traits, pq.Array(&walletIDs), &user.Universal, &user.CreationTime, &user.LastUpdated)
	if err != nil {
		if err == sql.ErrNoRows {
			return persist.User{}, persist.ErrUserNotFound{UserID: pID}
		}
		return persist.User{}, err
	}
	wallets := make([]persist.Wallet, len(walletIDs))

	for i, walletID := range walletIDs {
		wallet := persist.Wallet{ID: walletID}
		err = u.getWalletStmt.QueryRowContext(pCtx, walletID).Scan(&wallet.Address, &wallet.Chain, &wallet.WalletType, &wallet.Version, &wallet.CreationTime, &wallet.LastUpdated)
		if err != nil {
			return persist.User{}, fmt.Errorf("failed to get wallet: %w", err)
		}
		wallets[i] = wallet
	}
	user.Wallets = wallets

	return user, nil
}

// GetByIDs gets all the users with the given IDs
func (u *UserRepository) GetByIDs(pCtx context.Context, pIDs []persist.DBID) ([]persist.User, error) {

	results := make([]persist.User, 0, len(pIDs))
	rows, err := u.getByIDsStmt.QueryContext(pCtx, pIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		user := persist.User{}
		walletIDs := []persist.DBID{}
		rows.Scan(&user.ID, &user.Deleted, &user.Version, &user.Username, &user.UsernameIdempotent, &user.Bio, &user.Traits, pq.Array(&walletIDs), &user.Universal, &user.CreationTime, &user.LastUpdated)
		wallets := make([]persist.Wallet, len(walletIDs))

		for i, walletID := range walletIDs {
			wallet := persist.Wallet{ID: walletID}
			err = u.getWalletStmt.QueryRowContext(pCtx, walletID).Scan(&wallet.Address, &wallet.Chain, &wallet.WalletType, &wallet.Version, &wallet.CreationTime, &wallet.LastUpdated)
			if err != nil && err != sql.ErrNoRows {
				return nil, fmt.Errorf("failed to get wallet: %w", err)
			}
			wallets[i] = wallet
		}
		user.Wallets = wallets
		results = append(results, user)
	}

	return results, nil
}

// GetByChainAddress gets the user who owns the wallet with the specified ChainAddress (if any)
func (u *UserRepository) GetByChainAddress(pCtx context.Context, pChainAddress persist.ChainAddress) (persist.User, error) {
	var walletID persist.DBID

	err := u.getWalletIDStmt.QueryRowContext(pCtx, pChainAddress.Address(), pChainAddress.Chain()).Scan(&walletID)
	if err != nil {
		if err == sql.ErrNoRows {
			return persist.User{}, persist.ErrWalletNotFound{ChainAddress: pChainAddress}
		}
		return persist.User{}, err
	}

	return u.GetByWalletID(pCtx, walletID)
}

// GetByWalletID gets the user with the given wallet in their list of addresses
func (u *UserRepository) GetByWalletID(pCtx context.Context, pWalletID persist.DBID) (persist.User, error) {

	var user persist.User
	err := u.getByWalletIDStmt.QueryRowContext(pCtx, pWalletID).Scan(&user.ID, &user.Deleted, &user.Version, &user.Username, &user.UsernameIdempotent, pq.Array(&user.Wallets), &user.Bio, &user.Traits, &user.Universal, &user.CreationTime, &user.LastUpdated)
	if err != nil {
		if err == sql.ErrNoRows {
			return persist.User{}, persist.ErrUserNotFound{WalletID: pWalletID}
		}
		return persist.User{}, err
	}

	return user, nil

}

// GetByUsername gets the user with the given username
func (u *UserRepository) GetByUsername(pCtx context.Context, pUsername string) (persist.User, error) {

	var user persist.User
	err := u.getByUsernameStmt.QueryRowContext(pCtx, strings.ToLower(pUsername)).Scan(&user.ID, &user.Deleted, &user.Version, &user.Username, &user.UsernameIdempotent, pq.Array(&user.Wallets), &user.Bio, &user.Traits, &user.Universal, &user.CreationTime, &user.LastUpdated)
	if err != nil {
		if err == sql.ErrNoRows {
			return persist.User{}, persist.ErrUserNotFound{Username: pUsername}
		}
		return persist.User{}, err
	}
	return user, nil

}

// AddWallet adds an address to user as well as ensures that the wallet and address exists
func (u *UserRepository) AddWallet(pCtx context.Context, pUserID persist.DBID, pChainAddress persist.ChainAddress, pWalletType persist.WalletType) error {
	tx, err := u.db.BeginTx(pCtx, nil)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	walletID, err := u.createWalletWithTx(pCtx, tx, pChainAddress, pWalletType)
	if err != nil {
		return err
	}

	// Add the new wallet to the user
	if _, err := tx.StmtContext(pCtx, u.addWalletStmt).ExecContext(pCtx, walletID, pUserID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

// RemoveWallet removes an address from user
func (u *UserRepository) RemoveWallet(pCtx context.Context, pUserID persist.DBID, pWalletID persist.DBID) error {
	tx, err := u.db.BeginTx(pCtx, nil)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	if _, err := tx.StmtContext(pCtx, u.removeWalletFromUserStmt).ExecContext(pCtx, pWalletID, pUserID); err != nil {
		return err
	}

	if _, err := tx.StmtContext(pCtx, u.deleteWalletStmt).ExecContext(pCtx, pWalletID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
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
	if err := u.getByIDStmt.QueryRowContext(pCtx, pInitialUser).Scan(&user.ID, &user.Deleted, &user.Version, &user.Username, &user.UsernameIdempotent, pq.Array(&user.Wallets), &user.Bio, &user.Universal, &user.CreationTime, &user.LastUpdated); err != nil {
		return err
	}

	var secondUser persist.User
	if err := u.getByIDStmt.QueryRowContext(pCtx, pSecondUser).Scan(&secondUser.ID, &secondUser.Deleted, &secondUser.Version, &secondUser.Username, &secondUser.UsernameIdempotent, pq.Array(&secondUser.Wallets), &secondUser.Bio, &user.Universal, &secondUser.CreationTime, &secondUser.LastUpdated); err != nil {
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

func (u *UserRepository) AddFollower(pCtx context.Context, follower persist.DBID, followee persist.DBID) (refollowed bool, err error) {
	err = u.addFollowerStmt.QueryRowContext(pCtx, persist.GenerateID(), follower, followee).Scan(&refollowed)
	if err != nil {
		return false, err
	}

	return refollowed, nil
}

func (u *UserRepository) RemoveFollower(pCtx context.Context, follower persist.DBID, followee persist.DBID) error {
	_, err := u.removeFollowerStmt.ExecContext(pCtx, follower, followee)
	return err
}

func (u *UserRepository) UserFollowsUser(pCtx context.Context, follower persist.DBID, followee persist.DBID) (bool, error) {
	res, err := u.followsUserStmt.QueryContext(pCtx, follower, followee)
	if err != nil {
		return false, err
	}
	defer res.Close()

	var follows bool
	for res.Next() {
		err = res.Scan(&follows)
		if err != nil {
			return false, err
		}
	}

	if err = res.Err(); err != nil {
		return false, err
	}

	return follows, nil
}
func (u *UserRepository) FillWalletDataForUser(pCtx context.Context, user *persist.User) error {

	if len(user.Wallets) == 0 {
		return nil
	}

	wallets := make([]persist.Wallet, 0, len(user.Wallets))
	for _, wallet := range user.Wallets {
		wallet := persist.Wallet{ID: wallet.ID}
		if err := u.getWalletStmt.QueryRowContext(pCtx, wallet.ID).Scan(&wallet.Address, &wallet.Chain, &wallet.WalletType, &wallet.Version, &wallet.CreationTime, &wallet.LastUpdated); err != nil {
			return err
		}

		wallets = append(wallets, wallet)
	}

	user.Wallets = wallets

	return nil
}
