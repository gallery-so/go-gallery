package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

// CollectionTokenRepository is the repository for interacting with collections in a postgres database
type CollectionTokenRepository struct {
	db *sql.DB
}

// NewCollectionTokenRepository creates a new CollectionTokenRepository
func NewCollectionTokenRepository(db *sql.DB) *CollectionTokenRepository {
	return &CollectionTokenRepository{db: db}
}

// Create creates a new collection in the database
func (c *CollectionTokenRepository) Create(pCtx context.Context, pColl persist.CollectionTokenDB) (persist.DBID, error) {
	sqlStr := `INSERT INTO collections (ID, VERSION, NAME, COLLECTORS_NOTE, OWNER_USER_ID, LAYOUT, NFTS) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING ID;`

	var id string
	err := c.db.QueryRowContext(pCtx, sqlStr, persist.GenerateID(), pColl.Version, pColl.Name, pColl.CollectorsNote, pColl.OwnerUserID, pColl.Layout, pq.Array(pColl.NFTs)).Scan(&id)
	if err != nil {
		return "", err
	}
	return persist.DBID(id), nil
}

// GetByUserID returns all collections owned by a user
func (c *CollectionTokenRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID, pShowHidden bool) ([]persist.CollectionToken, error) {
	sqlStr := `SELECT c.ID,c.OWNER_USER_ID,c.NAME,c.VERSION,c.DELETED,c.COLLECTORS_NOTE,
		c.LAYOUT,c.CREATED_AT,c.LAST_UPDATED,
		n.ID,n.OWNER_ADDRESS,n.CHAIN,n.NAME,n.DESCRIPTION,n.TOKEN_TYPE,n.TOKEN_URI,n.TOKEN_ID,n.MEDIA,n.TOKEN_METADATA,n.CONTRACT_ADDRESS,n.CREATED_AT 
		FROM collections c 
		JOIN tokens n ON n.ID = ANY (c.NFTS) 
		WHERE c.OWNER_USER_ID = $1 AND c.HIDDEN = $2 GROUP BY c.ID,n.ID;`

	res, err := c.db.QueryContext(pCtx, sqlStr, pUserID, !pShowHidden)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	collections := make(map[persist.DBID]persist.CollectionToken)
	for res.Next() {
		var collection persist.CollectionToken
		var nft persist.TokenInCollection
		err = res.Scan(&collection.ID, &collection.OwnerUserID, &collection.Name, &collection.Version, &collection.Deleted, &collection.CollectorsNote, &collection.Layout, &collection.CreationTime, &collection.LastUpdated, &nft.ID, &nft.OwnerAddress, &nft.Chain, &nft.Name, &nft.Description, &nft.TokenType, &nft.TokenURI, &nft.TokenID, &nft.Media, &nft.TokenMetadata, &nft.ContractAddress, &nft.CreationTime)
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
	for _, collection := range collections {
		result = append(result, collection)
	}

	return result, nil
}

// GetByID returns a collection by its ID
func (c *CollectionTokenRepository) GetByID(pCtx context.Context, pID persist.DBID, pShowHidden bool) (persist.CollectionToken, error) {

	sqlStr := `SELECT c.ID,c.OWNER_USER_ID,c.NAME,c.VERSION,c.DELETED,c.COLLECTORS_NOTE,
		c.LAYOUT,c.CREATED_AT,c.LAST_UPDATED,
		n.ID,n.OWNER_ADDRESS,n.CHAIN,n.NAME,n.DESCRIPTION,n.TOKEN_TYPE,n.TOKEN_URI,n.TOKEN_ID,n.MEDIA,n.TOKEN_METADATA,n.CONTRACT_ADDRESS,n.CREATED_AT 
		FROM collections c 
		JOIN nfts n ON n.ID = ANY (c.NFTS) 
		WHERE c.ID = $1 AND c.HIDDEN = $2 GROUP BY c.ID,n.ID;`

	res, err := c.db.QueryContext(pCtx, sqlStr, pID, !pShowHidden)
	if err != nil {
		return persist.CollectionToken{}, err
	}
	defer res.Close()

	var collection persist.CollectionToken
	var nfts []persist.TokenInCollection

	for res.Next() {
		colID := collection.ID
		var nft persist.TokenInCollection
		err = res.Scan(&colID, &collection.OwnerUserID, &collection.Name, &collection.Version, &collection.Deleted, &collection.CollectorsNote, &collection.Layout, &collection.CreationTime, &collection.LastUpdated, &nft.ID, &nft.OwnerAddress, &nft.Chain, &nft.Name, &nft.Description, &nft.TokenType, &nft.TokenURI, &nft.TokenID, &nft.Media, &nft.TokenMetadata, &nft.ContractAddress, &nft.CreationTime)
		if err != nil {
			return persist.CollectionToken{}, err
		}
		if colID != "" && colID != collection.ID {
			return persist.CollectionToken{}, errors.New("multiple collections found")
		}

		nfts = append(nfts, nft)
	}
	if err := res.Err(); err != nil {
		return persist.CollectionToken{}, err
	}
	collection.NFTs = nfts

	return collection, nil
}

// Update updates a collection in the database
func (c *CollectionTokenRepository) Update(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {
	sqlStr := `UPDATE COLLECTIONS `
	switch pUpdate.(type) {
	case persist.CollectionTokenUpdateDeletedInput:
		update := pUpdate.(persist.CollectionTokenUpdateDeletedInput)
		sqlStr += `SET DELETED = $1, LAST_UPDATED = $2 WHERE ID = $3 AND OWNER_USER_ID = $4`
		_, err := c.db.ExecContext(pCtx, sqlStr, update.Deleted, time.Now(), pID, pUserID)
		return err
	case persist.CollectionTokenUpdateInfoInput:
		update := pUpdate.(persist.CollectionTokenUpdateInfoInput)
		sqlStr += `SET COLLECTORS_NOTE = $1, NAME = $2, LAST_UPDATED = $3 WHERE ID = $4 AND OWNER_USER_ID = $5`
		_, err := c.db.ExecContext(pCtx, sqlStr, update.CollectorsNote, update.Name, time.Now(), pID, pUserID)
		return err
	case persist.CollectionTokenUpdateHiddenInput:
		update := pUpdate.(persist.CollectionTokenUpdateHiddenInput)
		sqlStr += `SET HIDDEN = $1, LAST_UPDATED = $2 WHERE ID = $3 AND OWNER_USER_ID = $4`
		_, err := c.db.ExecContext(pCtx, sqlStr, update.Hidden, time.Now(), pID, pUserID)
		return err
	default:
		return errors.New("invalid update type")
	}
}

// UpdateNFTs updates the nfts of a collection in the database
func (c *CollectionTokenRepository) UpdateNFTs(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate persist.CollectionTokenUpdateNftsInput) error {
	sqlStr := `UPDATE collections SET NFTS = $1 WHERE ID = $2 AND OWNER_USER_ID = $3;`
	_, err := c.db.ExecContext(pCtx, sqlStr, pUpdate.Nfts, pID, pUserID)
	return err
}

// UpdateUnsafe updates a collection in the database
func (c *CollectionTokenRepository) UpdateUnsafe(pCtx context.Context, pID persist.DBID, pUpdate interface{}) error {
	sqlStr := `UPDATE COLLECTIONS `
	switch pUpdate.(type) {
	case persist.CollectionTokenUpdateDeletedInput:
		update := pUpdate.(persist.CollectionTokenUpdateDeletedInput)
		sqlStr += `SET DELETED = $1, LAST_UPDATED = $2 WHERE ID = $3;`
		_, err := c.db.ExecContext(pCtx, sqlStr, update.Deleted, time.Now(), pID)
		return err
	case persist.CollectionTokenUpdateInfoInput:
		update := pUpdate.(persist.CollectionTokenUpdateInfoInput)
		sqlStr += `SET COLLECTORS_NOTE = $1, NAME = $2, LAST_UPDATED = $3 WHERE ID = $4;`
		_, err := c.db.ExecContext(pCtx, sqlStr, update.CollectorsNote, update.Name, time.Now(), pID)
		return err
	case persist.CollectionTokenUpdateHiddenInput:
		update := pUpdate.(persist.CollectionTokenUpdateHiddenInput)
		sqlStr += `SET HIDDEN = $1, LAST_UPDATED = $2 WHERE ID = $3;`
		_, err := c.db.ExecContext(pCtx, sqlStr, update.Hidden, time.Now(), pID)
		return err
	default:
		return errors.New("invalid update type")
	}
}

// UpdateNFTsUnsafe updates the nfts of a collection in the database
func (c *CollectionTokenRepository) UpdateNFTsUnsafe(pCtx context.Context, pID persist.DBID, pUpdate persist.CollectionTokenUpdateNftsInput) error {
	sqlStr := `UPDATE collections SET NFTS = $1 WHERE ID = $2;`
	_, err := c.db.ExecContext(pCtx, sqlStr, pUpdate.Nfts, pID)
	return err
}

// ClaimNFTs claims nfts from a collection in the database
func (c *CollectionTokenRepository) ClaimNFTs(pCtx context.Context, pID persist.DBID, pOwnerAddresses []persist.Address, pUpdate persist.CollectionTokenUpdateNftsInput) error {
	nftsToRemoveSQLStr := `SELECT ID FROM nfts WHERE OWNER_ADDRESS = ANY($1) AND ID <> ALL($2);`
	nftsToRemove, err := c.db.QueryContext(pCtx, nftsToRemoveSQLStr, pq.Array(pOwnerAddresses), pq.Array(pUpdate.Nfts))
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

	deleteNFTsSQLStr := `UPDATE nfts SET DELETED = true WHERE ID = ANY($1);`
	_, err = c.db.ExecContext(pCtx, deleteNFTsSQLStr, pq.Array(nftsToRemoveIDs))
	if err != nil {
		return err
	}

	removeFromNFTsSQLStr := `UPDATE collections SET NFTS = array_remove(NFTS, ANY($1)) WHERE ID = $2;`
	_, err = c.db.ExecContext(pCtx, removeFromNFTsSQLStr, pq.Array(nftsToRemoveIDs), pID)
	if err != nil {
		return err
	}

	return nil

}

// RemoveNFTsOfAddresses removes nfts of addresses from a collection in the database
func (c *CollectionTokenRepository) RemoveNFTsOfAddresses(pCtx context.Context, pID persist.DBID, pAddresses []persist.Address) error {
	findNFTsForAddressesSQLStr := `SELECT ID FROM nfts WHERE OWNER_ADDRESS = ANY($1);`
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

	deleteNFTsSQLStr := `UPDATE nfts SET DELETED = true WHERE ID = ANY($1);`
	_, err = c.db.ExecContext(pCtx, deleteNFTsSQLStr, pq.Array(nftsIDs))
	if err != nil {
		return err
	}

	removeFromNFTsSQLStr := `UPDATE collections SET NFTS = array_remove(NFTS, ANY($1)) WHERE ID = $2;`
	_, err = c.db.ExecContext(pCtx, removeFromNFTsSQLStr, pq.Array(nftsIDs), pID)
	if err != nil {
		return err
	}

	return nil
}

// Delete deletes a collection from the database
func (c *CollectionTokenRepository) Delete(pCtx context.Context, pID persist.DBID, pUserID persist.DBID) error {
	sqlStr := `UPDATE collections SET DELETED = true WHERE ID = $1 AND OWNER_USER_ID = $2;`
	_, err := c.db.ExecContext(pCtx, sqlStr, pID, pUserID)
	return err
}

// GetUnassigned returns all unassigned nfts
func (c *CollectionTokenRepository) GetUnassigned(context.Context, persist.DBID) (persist.CollectionToken, error) {
	return persist.CollectionToken{}, nil
}

// RefreshUnassigned refreshes the unassigned nfts
func (c *CollectionTokenRepository) RefreshUnassigned(context.Context, persist.DBID) error {
	return nil
}
