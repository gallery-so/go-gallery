package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

var errNotOwnedByUser = errors.New("not all nfts are owned by the user")

// CollectionTokenRepository is the repository for interacting with collections in a postgres database
type CollectionTokenRepository struct {
	db                      *sql.DB
	galleryRepo             *GalleryTokenRepository
	createStmt              *sql.Stmt
	getByUserIDOwnerStmt    *sql.Stmt
	getByUserIDOwnerRawStmt *sql.Stmt

	getByIDOwnerStmt    *sql.Stmt
	getByIDOwnerRawStmt *sql.Stmt

	updateInfoStmt               *sql.Stmt
	updateInfoUnsafeStmt         *sql.Stmt
	updateHiddenStmt             *sql.Stmt
	updateHiddenUnsafeStmt       *sql.Stmt
	updateNFTsStmt               *sql.Stmt
	updateNFTsUnsafeStmt         *sql.Stmt
	nftsToRemoveStmt             *sql.Stmt
	deleteNFTsStmt               *sql.Stmt
	removeNFTFromCollectionsStmt *sql.Stmt
	getNFTsForAddressStmt        *sql.Stmt
	deleteCollectionStmt         *sql.Stmt
	getUserWalletsStmt           *sql.Stmt
	checkOwnNFTsStmt             *sql.Stmt
}

// NewCollectionTokenRepository creates a new CollectionTokenRepository
// TODO another join for addresses
func NewCollectionTokenRepository(db *sql.DB, galleryRepo *GalleryTokenRepository) *CollectionTokenRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx, `INSERT INTO collections_v2 (ID, VERSION, NAME, COLLECTORS_NOTE, OWNER_USER_ID, LAYOUT, NFTS, HIDDEN) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING ID;`)
	checkNoErr(err)

	getByUserIDOwnerStmt, err := db.PrepareContext(ctx, `SELECT c.ID,c.OWNER_USER_ID,c.NAME,c.VERSION,c.COLLECTORS_NOTE,
		c.LAYOUT,c.HIDDEN,c.CREATED_AT,c.LAST_UPDATED,
		n.ID,n.OWNER_USER_ID,n.CHAIN,n.NAME,n.DESCRIPTION,n.TOKEN_TYPE,n.TOKEN_URI,n.TOKEN_ID,n.MEDIA,n.TOKEN_METADATA,n.CONTRACT_ADDRESS,n.CREATED_AT 
		FROM collections_v2 c, unnest(c.NFTS) WITH ORDINALITY AS u(nft, ordinality) 
		JOIN tokens n ON n.ID = nft
		WHERE c.OWNER_USER_ID = $1 AND c.DELETED = false AND n.DELETED = false ORDER BY ordinality;`)
	checkNoErr(err)
	getByUserIDOwnerRawStmt, err := db.PrepareContext(ctx, `SELECT c.ID,c.OWNER_USER_ID,c.NAME,c.VERSION,c.DELETED,c.COLLECTORS_NOTE,c.LAYOUT,c.HIDDEN,c.CREATED_AT,c.LAST_UPDATED FROM collections_v2 c WHERE c.OWNER_USER_ID = $1 AND c.DELETED = false;`)
	checkNoErr(err)

	getByIDOwnerStmt, err := db.PrepareContext(ctx, `SELECT c.ID,c.OWNER_USER_ID,c.NAME,c.VERSION,c.COLLECTORS_NOTE,
	c.LAYOUT,c.HIDDEN,c.CREATED_AT,c.LAST_UPDATED,
	n.ID,n.OWNER_USER_ID,n.CHAIN,n.NAME,n.DESCRIPTION,n.TOKEN_TYPE,n.TOKEN_URI,n.TOKEN_ID,n.MEDIA,n.TOKEN_METADATA,n.CONTRACT_ADDRESS,n.CREATED_AT 
	FROM collections_v2 c, unnest(c.NFTS) WITH ORDINALITY AS u(nft, ordinality) 
	JOIN tokens n ON n.ID = nft
	WHERE c.ID = $1 AND c.DELETED = false AND n.DELETED = false ORDER BY ordinality;`)
	checkNoErr(err)

	getByIDOwnerRawStmt, err := db.PrepareContext(ctx, `SELECT c.ID,c.OWNER_USER_ID,c.NAME,c.VERSION,c.DELETED,c.COLLECTORS_NOTE,c.LAYOUT,c.HIDDEN,c.CREATED_AT,c.LAST_UPDATED FROM collections_v2 c WHERE c.ID = $1 AND c.DELETED = false;`)
	checkNoErr(err)

	updateInfoStmt, err := db.PrepareContext(ctx, `UPDATE collections_v2 SET COLLECTORS_NOTE = $1, NAME = $2, LAST_UPDATED = $3 WHERE ID = $4 AND OWNER_USER_ID = $5;`)
	checkNoErr(err)
	updateInfoUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE collections_v2 SET COLLECTORS_NOTE = $1, NAME = $2, LAST_UPDATED = $3 WHERE ID = $4;`)
	checkNoErr(err)

	updateHiddenStmt, err := db.PrepareContext(ctx, `UPDATE collections_v2 SET HIDDEN = $1, LAST_UPDATED = $2 WHERE ID = $3 AND OWNER_USER_ID = $4;`)
	checkNoErr(err)

	updateHiddenUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE collections_v2 SET HIDDEN = $1, LAST_UPDATED = $2 WHERE ID = $3;`)
	checkNoErr(err)

	updateNFTsStmt, err := db.PrepareContext(ctx, `UPDATE collections_v2 SET NFTS = $1, LAYOUT = $2, LAST_UPDATED = $3 WHERE ID = $4 AND OWNER_USER_ID = $5;`)
	checkNoErr(err)

	updateNFTsUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE collections_v2 SET NFTS = $1, LAYOUT = $2, LAST_UPDATED = $3 WHERE ID = $4;`)
	checkNoErr(err)

	nftsToRemoveStmt, err := db.PrepareContext(ctx, `SELECT ID FROM tokens WHERE OWNER_USER_ID = $1 AND ID <> ALL($2);`)
	checkNoErr(err)

	deleteNFTsStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET DELETED = true WHERE ID = ANY($1)`)
	checkNoErr(err)

	removeNFTFromCollectionsStmt, err := db.PrepareContext(ctx, `UPDATE collections_v2 SET NFTS = array_remove(NFTS, $1) WHERE OWNER_USER_ID = $2;`)
	checkNoErr(err)

	getNFTsForAddressStmt, err := db.PrepareContext(ctx, `SELECT ID FROM tokens WHERE OWNER_USER_ID = $1;`)
	checkNoErr(err)

	deleteCollectionStmt, err := db.PrepareContext(ctx, `UPDATE collections_v2 SET DELETED = true WHERE ID = $1 AND OWNER_USER_ID = $2;`)
	checkNoErr(err)

	getUserWalletsStmt, err := db.PrepareContext(ctx, `SELECT w.ID FROM users u, unnest(u.WALLETS) WITH ORDINALITY AS d(wallet, wallet_ord) LEFT JOIN wallets w on w.ID = wallet WHERE u.ID = $1;`)
	checkNoErr(err)

	checkOwnNFTsStmt, err := db.PrepareContext(ctx, `SELECT COUNT(*) FROM tokens WHERE OWNER_USER_ID = $1 AND ID = ANY($2);`)
	checkNoErr(err)

	return &CollectionTokenRepository{db: db, galleryRepo: galleryRepo, createStmt: createStmt, getByUserIDOwnerStmt: getByUserIDOwnerStmt, getByIDOwnerStmt: getByIDOwnerStmt, updateInfoStmt: updateInfoStmt, updateInfoUnsafeStmt: updateInfoUnsafeStmt, updateHiddenStmt: updateHiddenStmt, updateHiddenUnsafeStmt: updateHiddenUnsafeStmt, updateNFTsStmt: updateNFTsStmt, updateNFTsUnsafeStmt: updateNFTsUnsafeStmt, nftsToRemoveStmt: nftsToRemoveStmt, deleteNFTsStmt: deleteNFTsStmt, removeNFTFromCollectionsStmt: removeNFTFromCollectionsStmt, getNFTsForAddressStmt: getNFTsForAddressStmt, deleteCollectionStmt: deleteCollectionStmt, getUserWalletsStmt: getUserWalletsStmt, checkOwnNFTsStmt: checkOwnNFTsStmt, getByUserIDOwnerRawStmt: getByUserIDOwnerRawStmt, getByIDOwnerRawStmt: getByIDOwnerRawStmt}
}

// Create creates a new collection in the database
func (c *CollectionTokenRepository) Create(pCtx context.Context, pColl persist.CollectionTokenDB) (persist.DBID, error) {
	err := ensureTokensOwnedByUser(pCtx, c, pColl.OwnerUserID, pColl.NFTs)
	if err != nil {
		return "", err
	}
	var id persist.DBID
	err = c.createStmt.QueryRowContext(pCtx, persist.GenerateID(), pColl.Version, pColl.Name, pColl.CollectorsNote, pColl.OwnerUserID, pColl.Layout, pq.Array(pColl.NFTs), pColl.Hidden).Scan(&id)
	if err != nil {
		return "", err
	}
	if err := c.galleryRepo.RefreshCache(pCtx, pColl.OwnerUserID); err != nil {
		return "", err
	}
	return id, nil
}

// GetByUserID returns all collections owned by a user
func (c *CollectionTokenRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID) ([]persist.CollectionToken, error) {
	stmt := c.getByUserIDOwnerStmt
	rawStmt := c.getByUserIDOwnerRawStmt

	res, err := stmt.QueryContext(pCtx, pUserID)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	collections := make(map[persist.DBID]persist.CollectionToken)
	for res.Next() {
		var collection persist.CollectionToken
		var nft persist.TokenInCollection
		err = res.Scan(&collection.ID, &collection.OwnerUserID, &collection.Name, &collection.Version, &collection.CollectorsNote, &collection.Layout, &collection.Hidden, &collection.CreationTime, &collection.LastUpdated, &nft.ID, &nft.OwnerAddress, &nft.Chain, &nft.Name, &nft.Description, &nft.TokenType, &nft.TokenURI, &nft.TokenID, &nft.Media, &nft.TokenMetadata, &nft.ContractAddress, &nft.CreationTime)
		if err != nil {
			return nil, err
		}

		if coll, ok := collections[collection.ID]; !ok {
			collection.NFTs = []persist.TokenInCollection{nft}
			collections[collection.ID] = collection
		} else {
			coll.NFTs = append(coll.NFTs, nft)
			collections[collection.ID] = coll
		}
	}

	if err := res.Err(); err != nil {
		return nil, err
	}

	result := make([]persist.CollectionToken, 0, len(collections))

	if len(collections) == 0 {
		colls, err := rawStmt.QueryContext(pCtx, pUserID)
		if err != nil {
			return nil, err
		}
		defer colls.Close()
		for colls.Next() {
			var rawColl persist.CollectionToken
			err = colls.Scan(&rawColl.ID, &rawColl.OwnerUserID, &rawColl.Name, &rawColl.Version, &rawColl.Deleted, &rawColl.CollectorsNote, &rawColl.Layout, &rawColl.Hidden, &rawColl.CreationTime, &rawColl.LastUpdated)
			if err != nil {
				return nil, err
			}
			rawColl.NFTs = []persist.TokenInCollection{}
			result = append(result, rawColl)
		}
		if err := colls.Err(); err != nil {
			return nil, err
		}
		return result, nil
	}

	for _, collection := range collections {
		result = append(result, collection)
	}

	return result, nil
}

// GetByID returns a collection by its ID
func (c *CollectionTokenRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.CollectionToken, error) {
	stmt := c.getByIDOwnerStmt
	rawStmt := c.getByIDOwnerRawStmt

	res, err := stmt.QueryContext(pCtx, pID)
	if err != nil {
		return persist.CollectionToken{}, err
	}
	defer res.Close()

	var collection persist.CollectionToken
	nfts := make([]persist.TokenInCollection, 0, 10)

	for res.Next() {
		colID := collection.ID
		var nft persist.TokenInCollection
		err = res.Scan(&collection.ID, &collection.OwnerUserID, &collection.Name, &collection.Version, &collection.CollectorsNote, &collection.Layout, &collection.Hidden, &collection.CreationTime, &collection.LastUpdated, &nft.ID, &nft.OwnerAddress, &nft.Chain, &nft.Name, &nft.Description, &nft.TokenType, &nft.TokenURI, &nft.TokenID, &nft.Media, &nft.TokenMetadata, &nft.ContractAddress, &nft.CreationTime)
		if err != nil {
			return persist.CollectionToken{}, err
		}
		if colID != "" && colID != collection.ID {
			return persist.CollectionToken{}, fmt.Errorf("mismatched coll ids colID: %s, collection.ID: %s", colID, collection.ID)
		}

		nfts = append(nfts, nft)
	}
	if err := res.Err(); err != nil {
		return persist.CollectionToken{}, err
	}

	if collection.ID == "" {
		collection.NFTs = []persist.TokenInCollection{}
		err := rawStmt.QueryRowContext(pCtx, pID).Scan(&collection.ID, &collection.OwnerUserID, &collection.Name, &collection.Version, &collection.Deleted, &collection.CollectorsNote, &collection.Layout, &collection.Hidden, &collection.CreationTime, &collection.LastUpdated)
		if err != nil {
			if err == sql.ErrNoRows {
				return persist.CollectionToken{}, persist.ErrCollectionNotFoundByID{ID: pID}
			}
			return persist.CollectionToken{}, err
		}
		if collection.ID != pID {
			return persist.CollectionToken{}, persist.ErrCollectionNotFoundByID{ID: pID}
		}
		return collection, nil
	}

	collection.NFTs = nfts

	return collection, nil
}

// Update updates a collection in the database
func (c *CollectionTokenRepository) Update(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {
	var res sql.Result
	var err error
	switch pUpdate.(type) {
	case persist.CollectionTokenUpdateInfoInput:
		update := pUpdate.(persist.CollectionTokenUpdateInfoInput)
		res, err = c.updateInfoStmt.ExecContext(pCtx, update.CollectorsNote, update.Name, time.Now(), pID, pUserID)
	case persist.CollectionTokenUpdateHiddenInput:
		update := pUpdate.(persist.CollectionTokenUpdateHiddenInput)
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
	return c.galleryRepo.RefreshCache(pCtx, pUserID)
}

// UpdateNFTs updates the nfts of a collection in the database
func (c *CollectionTokenRepository) UpdateNFTs(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate persist.CollectionTokenUpdateNftsInput) error {
	err := ensureTokensOwnedByUser(pCtx, c, pUserID, pUpdate.NFTs)
	if err != nil {
		return err
	}
	res, err := c.updateNFTsStmt.ExecContext(pCtx, pq.Array(pUpdate.NFTs), pUpdate.Layout, time.Now(), pID, pUserID)
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
	return c.galleryRepo.RefreshCache(pCtx, pUserID)
}

// UpdateUnsafe updates a collection in the database
func (c *CollectionTokenRepository) UpdateUnsafe(pCtx context.Context, pID persist.DBID, pUpdate interface{}) error {
	var res sql.Result
	var err error
	switch pUpdate.(type) {
	case persist.CollectionTokenUpdateInfoInput:
		update := pUpdate.(persist.CollectionTokenUpdateInfoInput)
		res, err = c.updateInfoUnsafeStmt.ExecContext(pCtx, update.CollectorsNote, update.Name, time.Now(), pID)
	case persist.CollectionTokenUpdateHiddenInput:
		update := pUpdate.(persist.CollectionTokenUpdateHiddenInput)
		res, err = c.updateHiddenUnsafeStmt.ExecContext(pCtx, update.Hidden, time.Now(), pID)
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

// UpdateNFTsUnsafe updates the nfts of a collection in the database
func (c *CollectionTokenRepository) UpdateNFTsUnsafe(pCtx context.Context, pID persist.DBID, pUpdate persist.CollectionTokenUpdateNftsInput) error {
	res, err := c.updateNFTsUnsafeStmt.ExecContext(pCtx, pq.Array(pUpdate.NFTs), pUpdate.Layout, time.Now(), pID)
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

// ClaimNFTs claims nfts from a collection in the database
func (c *CollectionTokenRepository) ClaimNFTs(pCtx context.Context, pUserID persist.DBID, pOwnerAddresses []persist.EthereumAddress, pUpdate persist.CollectionTokenUpdateNftsInput) error {
	nftsToRemove, err := c.nftsToRemoveStmt.QueryContext(pCtx, pq.Array(pOwnerAddresses), pq.Array(pUpdate.NFTs))
	if err != nil {
		return err
	}
	defer nftsToRemove.Close()

	nftsToRemoveIDs := []persist.DBID{}
	for nftsToRemove.Next() {
		var id persist.DBID
		err = nftsToRemove.Scan(&id)
		if err != nil {
			return err
		}
		nftsToRemoveIDs = append(nftsToRemoveIDs, id)
	}

	if err := nftsToRemove.Err(); err != nil {
		return err
	}

	_, err = c.deleteNFTsStmt.ExecContext(pCtx, pq.Array(nftsToRemoveIDs))
	if err != nil {
		return err
	}

	for _, nft := range nftsToRemoveIDs {
		_, err = c.removeNFTFromCollectionsStmt.ExecContext(pCtx, nft, pUserID)
		if err != nil {
			return err
		}
	}

	return c.galleryRepo.RefreshCache(pCtx, pUserID)

}

// RemoveNFTsOfAddresses removes nfts of addresses from a collection in the database
func (c *CollectionTokenRepository) RemoveNFTsOfAddresses(pCtx context.Context, pID persist.DBID, pAddresses []persist.EthereumAddress) error {
	nfts, err := c.getNFTsForAddressStmt.QueryContext(pCtx, pq.Array(pAddresses))
	if err != nil {
		return err
	}
	defer nfts.Close()

	nftsIDs := []persist.DBID{}
	for nfts.Next() {
		var id persist.DBID
		err = nfts.Scan(&id)
		if err != nil {
			return err
		}
		nftsIDs = append(nftsIDs, id)
	}

	if err := nfts.Err(); err != nil {
		return err
	}

	_, err = c.deleteNFTsStmt.ExecContext(pCtx, pq.Array(nftsIDs))
	if err != nil {
		return err
	}

	for _, nft := range nftsIDs {
		_, err := c.removeNFTFromCollectionsStmt.ExecContext(pCtx, nft, pID)
		if err != nil {
			return err
		}
	}

	return nil
}

// RemoveNFTsOfOldAddresses removes nfts of addresses that a user no longer has
func (c *CollectionTokenRepository) RemoveNFTsOfOldAddresses(pCtx context.Context, pUserID persist.DBID) error {
	colls, err := c.GetByUserID(pCtx, pUserID)
	if err != nil {
		return err
	}

	var walletIDs []persist.DBID
	if err := c.getUserWalletsStmt.QueryRowContext(pCtx, pUserID).Scan(pq.Array(&walletIDs)); err != nil {
		return err
	}

	for _, coll := range colls {
		for _, nft := range coll.NFTs {
			if !persist.ContainsDBID(walletIDs, nft.OwnerAddress) {
				_, err := c.removeNFTFromCollectionsStmt.ExecContext(pCtx, nft.ID, pUserID)
				if err != nil {
					return err
				}
			}
		}
	}

	return c.galleryRepo.RefreshCache(pCtx, pUserID)
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
	return c.galleryRepo.RefreshCache(pCtx, pUserID)
}

func ensureTokensOwnedByUser(pCtx context.Context, c *CollectionTokenRepository, pUserID persist.DBID, nfts []persist.DBID) error {
	var addresses []persist.EthereumAddress
	err := c.getUserWalletsStmt.QueryRowContext(pCtx, pUserID).Scan(pq.Array(&addresses))
	if err != nil {
		return err
	}

	var ct int64
	err = c.checkOwnNFTsStmt.QueryRowContext(pCtx, pq.Array(addresses), pq.Array(nfts)).Scan(&ct)
	if err != nil {
		return err
	}
	if ct != int64(len(nfts)) {
		return errNotOwnedByUser
	}
	return nil
}

func containsAddress(pStrings []persist.Address, pString persist.Address) bool {
	for _, s := range pStrings {
		if s == pString {
			return true
		}
	}
	return false
}
