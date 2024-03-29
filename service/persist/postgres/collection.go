package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

// CollectionTokenRepository is the repository for interacting with collections in a postgres database
type CollectionTokenRepository struct {
	db         *sql.DB
	queries    *db.Queries
	createStmt *sql.Stmt

	updateInfoStmt               *sql.Stmt
	updateInfoUnsafeStmt         *sql.Stmt
	updateHiddenStmt             *sql.Stmt
	updateHiddenUnsafeStmt       *sql.Stmt
	updateNFTsStmt               *sql.Stmt
	updateNFTsUnsafeStmt         *sql.Stmt
	nftsToRemoveStmt             *sql.Stmt
	removeNFTFromCollectionsStmt *sql.Stmt
	getNFTsForAddressStmt        *sql.Stmt
	deleteCollectionStmt         *sql.Stmt
	getUserWalletsStmt           *sql.Stmt
}

// NewCollectionTokenRepository creates a new CollectionTokenRepository
// TODO another join for addresses
func NewCollectionTokenRepository(db *sql.DB, queries *db.Queries) *CollectionTokenRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx, `INSERT INTO collections (ID, VERSION, NAME, COLLECTORS_NOTE, OWNER_USER_ID, GALLERY_ID, LAYOUT, NFTS, HIDDEN, TOKEN_SETTINGS) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING ID;`)
	checkNoErr(err)

	updateInfoStmt, err := db.PrepareContext(ctx, `UPDATE collections SET COLLECTORS_NOTE = $1, NAME = $2, LAST_UPDATED = $3 WHERE ID = $4 AND OWNER_USER_ID = $5;`)
	checkNoErr(err)
	updateInfoUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE collections SET COLLECTORS_NOTE = $1, NAME = $2, LAST_UPDATED = $3 WHERE ID = $4;`)
	checkNoErr(err)

	updateHiddenStmt, err := db.PrepareContext(ctx, `UPDATE collections SET HIDDEN = $1, LAST_UPDATED = $2 WHERE ID = $3 AND OWNER_USER_ID = $4;`)
	checkNoErr(err)

	updateHiddenUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE collections SET HIDDEN = $1, LAST_UPDATED = $2 WHERE ID = $3;`)
	checkNoErr(err)

	updateNFTsStmt, err := db.PrepareContext(ctx, `UPDATE collections SET NFTS = $1, LAYOUT = $2, LAST_UPDATED = $3, TOKEN_SETTINGS = $4, VERSION = $5 WHERE ID = $6 AND OWNER_USER_ID = $7;`)
	checkNoErr(err)

	updateNFTsUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE collections SET NFTS = $1, LAYOUT = $2, LAST_UPDATED = $3, TOKEN_SETTINGS = $4 WHERE ID = $5;`)
	checkNoErr(err)

	nftsToRemoveStmt, err := db.PrepareContext(ctx, `SELECT ID FROM tokens WHERE OWNER_USER_ID = $1 AND ID <> ALL($2);`)
	checkNoErr(err)

	removeNFTFromCollectionsStmt, err := db.PrepareContext(ctx, `UPDATE collections SET NFTS = array_remove(NFTS, $1) WHERE OWNER_USER_ID = $2;`)
	checkNoErr(err)

	getNFTsForAddressStmt, err := db.PrepareContext(ctx, `SELECT ID FROM tokens WHERE OWNER_USER_ID = $1;`)
	checkNoErr(err)

	deleteCollectionStmt, err := db.PrepareContext(ctx, `UPDATE collections SET DELETED = true WHERE ID = $1 AND OWNER_USER_ID = $2;`)
	checkNoErr(err)

	getUserWalletsStmt, err := db.PrepareContext(ctx, `SELECT wallets FROM users WHERE ID = $1;`)
	checkNoErr(err)

	return &CollectionTokenRepository{db: db, queries: queries, createStmt: createStmt, updateInfoStmt: updateInfoStmt, updateInfoUnsafeStmt: updateInfoUnsafeStmt, updateHiddenStmt: updateHiddenStmt, updateHiddenUnsafeStmt: updateHiddenUnsafeStmt, updateNFTsStmt: updateNFTsStmt, updateNFTsUnsafeStmt: updateNFTsUnsafeStmt, nftsToRemoveStmt: nftsToRemoveStmt, removeNFTFromCollectionsStmt: removeNFTFromCollectionsStmt, getNFTsForAddressStmt: getNFTsForAddressStmt, deleteCollectionStmt: deleteCollectionStmt, getUserWalletsStmt: getUserWalletsStmt}
}

// Create creates a new collection in the database
func (c *CollectionTokenRepository) Create(pCtx context.Context, pColl persist.CollectionDB) (persist.DBID, error) {
	var id persist.DBID
	err := c.createStmt.QueryRowContext(pCtx, persist.GenerateID(), pColl.Version, pColl.Name, pColl.CollectorsNote, pColl.OwnerUserID, pColl.GalleryID, pColl.Layout, pq.Array(pColl.Tokens), pColl.Hidden, pColl.TokenSettings).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

// Update updates a collection in the database
func (c *CollectionTokenRepository) Update(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {
	var res sql.Result
	var err error
	switch update := pUpdate.(type) {
	case persist.CollectionUpdateInfoInput:
		res, err = c.updateInfoStmt.ExecContext(pCtx, update.CollectorsNote, update.Name, time.Now(), pID, pUserID)
	case persist.CollectionUpdateHiddenInput:
		res, err = c.updateHiddenStmt.ExecContext(pCtx, update.Hidden, time.Now(), pID, pUserID)
	default:
		return fmt.Errorf("unsupported update type: %T", pUpdate)
	}
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return persist.ErrCollectionNotFoundByID{ID: pID}
	}
	return nil
}

// UpdateTokens updates the nfts of a collection in the database
func (c *CollectionTokenRepository) UpdateTokens(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate persist.CollectionUpdateTokensInput) error {
	res, err := c.updateNFTsStmt.ExecContext(pCtx, pq.Array(pUpdate.Tokens), pUpdate.Layout, time.Now(), pUpdate.TokenSettings, pUpdate.Version, pID, pUserID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return persist.ErrCollectionNotFoundByID{ID: pID}
	}
	return nil
}

// Delete deletes a collection from the database
func (c *CollectionTokenRepository) Delete(pCtx context.Context, pID persist.DBID, pUserID persist.DBID) error {
	res, err := c.deleteCollectionStmt.ExecContext(pCtx, pID, pUserID)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return persist.ErrCollectionNotFoundByID{ID: pID}
	}
	return nil
}
