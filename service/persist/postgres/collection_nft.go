package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

// CollectionRepository is the repository for interacting with collections in a postgres database
type CollectionRepository struct {
	db *sql.DB
}

// NewCollectionRepository creates a new CollectionRepository
func NewCollectionRepository(db *sql.DB) *CollectionRepository {
	return &CollectionRepository{db: db}
}

// Create creates a new collection in the database
func (c *CollectionRepository) Create(pCtx context.Context, pColl persist.CollectionDB) (persist.DBID, error) {
	sqlStr := `INSERT INTO collections (ID, VERSION, NAME, COLLECTORS_NOTE, OWNER_USER_ID, LAYOUT, NFTS, HIDDEN) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING ID;`

	var id persist.DBID
	err := c.db.QueryRowContext(pCtx, sqlStr, persist.GenerateID(), pColl.Version, pColl.Name, pColl.CollectorsNote, pColl.OwnerUserID, pColl.Layout, pq.Array(pColl.NFTs), pColl.Hidden).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

// GetByUserID returns all collections owned by a user
func (c *CollectionRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID, pShowHidden bool) ([]persist.Collection, error) {
	var sqlStr string
	if pShowHidden {
		sqlStr = `SELECT c.ID,c.OWNER_USER_ID,c.NAME,c.VERSION,c.DELETED,c.COLLECTORS_NOTE,
		c.LAYOUT,c.CREATED_AT,c.LAST_UPDATED,n.ID,n.OWNER_ADDRESS,
		n.MULTIPLE_OWNERS,n.NAME,n.CONTRACT,n.TOKEN_COLLECTION_NAME,n.CREATOR_ADDRESS,n.CREATOR_NAME, 
		n.IMAGE_URL,n.IMAGE_THUMBNAIL_URL,n.IMAGE_PREVIEW_URL,n.CREATED_AT 
		FROM collections c, unnest(c.NFTS) WITH ORDINALITY AS u(nft, ordinality)
		LEFT JOIN nfts n ON n.ID = nft
		WHERE c.OWNER_USER_ID = $1 AND c.DELETED = false ORDER BY ordinality;`
	} else {
		sqlStr = `SELECT c.ID,c.OWNER_USER_ID,c.NAME,c.VERSION,c.DELETED,c.COLLECTORS_NOTE,
		c.LAYOUT,c.CREATED_AT,c.LAST_UPDATED,n.ID,n.OWNER_ADDRESS,
		n.MULTIPLE_OWNERS,n.NAME,n.CONTRACT,n.TOKEN_COLLECTION_NAME,n.CREATOR_ADDRESS,n.CREATOR_NAME, 
		n.IMAGE_URL,n.IMAGE_THUMBNAIL_URL,n.IMAGE_PREVIEW_URL,n.CREATED_AT 
		FROM collections c,unnest(c.NFTS) WITH ORDINALITY AS u(nft, ordinality) 
		LEFT JOIN nfts n ON n.ID = nft 
		WHERE c.OWNER_USER_ID = $1 AND c.HIDDEN = false AND c.DELETED = false ORDER BY ordinality;`
	}
	res, err := c.db.QueryContext(pCtx, sqlStr, pUserID)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	collections := make(map[persist.DBID]persist.Collection)
	for res.Next() {
		var collection persist.Collection
		var nft persist.CollectionNFT
		err = res.Scan(&collection.ID, &collection.OwnerUserID, &collection.Name, &collection.Version, &collection.Deleted, &collection.CollectorsNote, &collection.Layout, &collection.CreationTime, &collection.LastUpdated, &nft.ID, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Name, &nft.Contract, &nft.TokenCollectionName, &nft.CreatorAddress, &nft.CreatorName, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.CreationTime)
		if err != nil {
			return nil, err
		}

		if coll, ok := collections[collection.ID]; !ok {
			collection.NFTs = []persist.CollectionNFT{nft}
			collections[collection.ID] = collection
		} else {
			coll.NFTs = append(coll.NFTs, nft)
			collections[collection.ID] = coll
		}
	}

	if err := res.Err(); err != nil {
		return nil, err
	}

	result := make([]persist.Collection, 0, len(collections))
	for _, collection := range collections {
		result = append(result, collection)
	}

	return result, nil
}

// GetByID returns a collection by its ID
func (c *CollectionRepository) GetByID(pCtx context.Context, pID persist.DBID, pShowHidden bool) (persist.Collection, error) {
	var sqlStr string
	// TODO if c.NFTS is empty no rows get returned :(
	if pShowHidden {
		sqlStr = `SELECT c.ID,c.OWNER_USER_ID,c.NAME,c.VERSION,c.DELETED,c.COLLECTORS_NOTE,
		c.LAYOUT,c.CREATED_AT,c.LAST_UPDATED,n.ID,n.OWNER_ADDRESS,
		n.MULTIPLE_OWNERS,n.NAME,n.CONTRACT,n.TOKEN_COLLECTION_NAME,n.CREATOR_ADDRESS,n.CREATOR_NAME, 
		n.IMAGE_URL,n.IMAGE_THUMBNAIL_URL,n.IMAGE_PREVIEW_URL,n.CREATED_AT 
		FROM collections c, unnest(c.NFTS) WITH ORDINALITY AS u(nft, ordinality)
		LEFT JOIN nfts n ON n.ID = nft
		WHERE c.ID = $1 AND c.DELETED = false ORDER BY ordinality;`
	} else {
		sqlStr = `SELECT c.ID,c.OWNER_USER_ID,c.NAME,c.VERSION,c.DELETED,c.COLLECTORS_NOTE,
		c.LAYOUT,c.CREATED_AT,c.LAST_UPDATED,n.ID,n.OWNER_ADDRESS,
		n.MULTIPLE_OWNERS,n.NAME,n.CONTRACT,n.TOKEN_COLLECTION_NAME,n.CREATOR_ADDRESS,n.CREATOR_NAME,
		n.IMAGE_URL,n.IMAGE_THUMBNAIL_URL,n.IMAGE_PREVIEW_URL,n.CREATED_AT 
		FROM collections c, unnest(c.NFTS) WITH ORDINALITY AS u(nft, ordinality)
		LEFT JOIN nfts n ON n.ID = nft
		WHERE c.ID = $1 AND c.HIDDEN = false AND c.DELETED = false ORDER BY ordinality;`
	}

	res, err := c.db.QueryContext(pCtx, sqlStr, pID)
	if err != nil {
		return persist.Collection{}, err
	}
	defer res.Close()

	var collection persist.Collection
	var nfts []persist.CollectionNFT
	i := 0
	for ; res.Next(); i++ {
		colID := collection.ID
		var nft persist.CollectionNFT
		err = res.Scan(&collection.ID, &collection.OwnerUserID, &collection.Name, &collection.Version, &collection.Deleted, &collection.CollectorsNote, &collection.Layout, &collection.CreationTime, &collection.LastUpdated, &nft.ID, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Name, &nft.Contract, &nft.TokenCollectionName, &nft.CreatorAddress, &nft.CreatorName, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.CreationTime)
		if err != nil {
			return persist.Collection{}, err
		}
		if colID != "" && colID != collection.ID {
			return persist.Collection{}, errors.New("multiple collections found")
		}

		nfts = append(nfts, nft)
	}
	if err := res.Err(); err != nil {
		return persist.Collection{}, err
	}
	if i == 0 {
		return persist.Collection{}, persist.ErrCollectionNotFoundByID{ID: pID}
	}

	collection.NFTs = nfts

	return collection, nil
}

// Update updates a collection in the database
func (c *CollectionRepository) Update(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {
	sqlStr := `UPDATE COLLECTIONS `
	var res sql.Result
	var err error
	switch pUpdate.(type) {
	case persist.CollectionUpdateDeletedInput:
		update := pUpdate.(persist.CollectionUpdateDeletedInput)
		sqlStr += `SET DELETED = $1, LAST_UPDATED = $2 WHERE ID = $3 AND OWNER_USER_ID = $4`
		res, err = c.db.ExecContext(pCtx, sqlStr, update.Deleted, time.Now(), pID, pUserID)
	case persist.CollectionUpdateInfoInput:
		update := pUpdate.(persist.CollectionUpdateInfoInput)
		sqlStr += `SET COLLECTORS_NOTE = $1, NAME = $2, LAST_UPDATED = $3 WHERE ID = $4 AND OWNER_USER_ID = $5`
		res, err = c.db.ExecContext(pCtx, sqlStr, update.CollectorsNote, update.Name, time.Now(), pID, pUserID)
	case persist.CollectionUpdateHiddenInput:
		update := pUpdate.(persist.CollectionUpdateHiddenInput)
		sqlStr += `SET HIDDEN = $1, LAST_UPDATED = $2 WHERE ID = $3 AND OWNER_USER_ID = $4`
		res, err = c.db.ExecContext(pCtx, sqlStr, update.Hidden, time.Now(), pID, pUserID)
	default:
		return errors.New("invalid update type")
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

// UpdateNFTs updates the nfts of a collection in the database
func (c *CollectionRepository) UpdateNFTs(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate persist.CollectionUpdateNftsInput) error {
	sqlStr := `UPDATE collections SET NFTS = $1, LAYOUT = $2, LAST_UPDATED = $3 WHERE ID = $4 AND OWNER_USER_ID = $5;`
	res, err := c.db.ExecContext(pCtx, sqlStr, pq.Array(pUpdate.NFTs), pUpdate.Layout, time.Now(), pID, pUserID)
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
func (c *CollectionRepository) ClaimNFTs(pCtx context.Context, pUserID persist.DBID, pOwnerAddresses []persist.Address, pUpdate persist.CollectionUpdateNftsInput) error {
	nftsToRemoveSQLStr := `SELECT ID,OPENSEA_ID FROM nfts WHERE OWNER_ADDRESS = ANY($1) AND ID <> ALL($2);`
	nftsToRemove, err := c.db.QueryContext(pCtx, nftsToRemoveSQLStr, pq.Array(pOwnerAddresses), pq.Array(pUpdate.NFTs))
	if err != nil {
		return err
	}
	defer nftsToRemove.Close()

	nftsToRemoveIDs := []persist.DBID{}
	for nftsToRemove.Next() {
		var id persist.DBID
		var openseaID int64

		err = nftsToRemove.Scan(&id, &openseaID)
		if err != nil {
			return err
		}
		nftsToRemoveIDs = append(nftsToRemoveIDs, id)
	}

	if err := nftsToRemove.Err(); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}

	deleteNFTsSQLStr := `UPDATE nfts SET DELETED = true WHERE ID = ANY($1);`
	_, err = c.db.ExecContext(pCtx, deleteNFTsSQLStr, pq.Array(nftsToRemoveIDs))
	if err != nil {
		return err
	}

	for _, nft := range nftsToRemoveIDs {
		removeFromNFTsSQLStr := `UPDATE collections SET NFTS = array_remove(NFTS, $1) WHERE OWNER_USER_ID = $2;`
		_, err := c.db.ExecContext(pCtx, removeFromNFTsSQLStr, nft, pUserID)
		if err != nil {
			return err
		}
	}

	return nil

}

// RemoveNFTsOfAddresses removes nfts of addresses from a collection in the database
func (c *CollectionRepository) RemoveNFTsOfAddresses(pCtx context.Context, pID persist.DBID, pAddresses []persist.Address) error {
	findNFTsForAddressesSQLStr := `SELECT ID FROM nfts WHERE OWNER_ADDRESS = ANY($1)`
	nfts, err := c.db.QueryContext(pCtx, findNFTsForAddressesSQLStr, pq.Array(pAddresses))
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

	deleteNFTsSQLStr := `UPDATE nfts SET DELETED = true WHERE ID = ANY($1)`
	_, err = c.db.ExecContext(pCtx, deleteNFTsSQLStr, pq.Array(nftsIDs))
	if err != nil {
		return err
	}

	for _, nft := range nftsIDs {
		removeFromNFTsSQLStr := `UPDATE collections SET NFTS = array_remove(NFTS, $1) WHERE OWNER_USER_ID = $2;`
		_, err = c.db.ExecContext(pCtx, removeFromNFTsSQLStr, nft, pID)
		if err != nil {
			return err
		}
	}

	return nil
}

// Delete deletes a collection from the database
func (c *CollectionRepository) Delete(pCtx context.Context, pID persist.DBID, pUserID persist.DBID) error {
	sqlStr := `UPDATE collections SET DELETED = true WHERE ID = $1 AND OWNER_USER_ID = $2;`
	res, err := c.db.ExecContext(pCtx, sqlStr, pID, pUserID)
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

// GetUnassigned returns all unassigned nfts
func (c *CollectionRepository) GetUnassigned(pCtx context.Context, pUserID persist.DBID) (persist.Collection, error) {

	getUserSQLStr := `SELECT ADDRESSES FROM users WHERE ID = $1;`
	var addresses []persist.Address
	err := c.db.QueryRowContext(pCtx, getUserSQLStr, pUserID).Scan(pq.Array(&addresses))

	// sql statement that gets all nfts that are not in any of a user's collections
	sqlStr := `SELECT n.ID,n.CREATED_AT,n.NAME,n.CREATOR_ADDRESS,n.CREATOR_NAME,n.OWNER_ADDRESS,n.MULTIPLE_OWNERS,n.CONTRACT,n.IMAGE_URL,n.IMAGE_THUMBNAIL_URL,n.IMAGE_PREVIEW_URL,n.TOKEN_COLLECTION_NAME 
	FROM nfts n
	JOIN collections c on n.ID <> ALL(c.NFTS)
	WHERE c.OWNER_USER_ID = $1 AND n.OWNER_ADDRESS = ANY($2);`
	rows, err := c.db.QueryContext(pCtx, sqlStr, pUserID, pq.Array(addresses))
	if err != nil {
		return persist.Collection{}, err
	}
	defer rows.Close()

	nfts := []persist.CollectionNFT{}
	for rows.Next() {
		var nft persist.CollectionNFT
		err = rows.Scan(&nft.ID, &nft.CreationTime, &nft.Name, &nft.CreatorAddress, &nft.CreatorName, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Contract, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.TokenCollectionName)
		if err != nil {
			return persist.Collection{}, err
		}
		nfts = append(nfts, nft)
	}

	if err := rows.Err(); err != nil {
		return persist.Collection{}, err
	}

	return persist.Collection{
		NFTs: nfts,
	}, nil

}

// RefreshUnassigned refreshes the unassigned nfts
func (c *CollectionRepository) RefreshUnassigned(context.Context, persist.DBID) error {
	return nil
}
