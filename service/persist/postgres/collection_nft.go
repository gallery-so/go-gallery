package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

// CollectionRepository is the repository for interacting with collections in a postgres database
type CollectionRepository struct {
	db *sql.DB
}

// NewCollectionRepository creates a new CollectionPostgresRepository
func NewCollectionRepository(db *sql.DB) *CollectionRepository {
	return &CollectionRepository{db: db}
}

// Create creates a new collection in the database
func (c *CollectionRepository) Create(pCtx context.Context, pColl persist.CollectionDB) (persist.DBID, error) {
	sqlStr := `INSERT INTO collections (ID, VERSION, NAME, COLLECTORS_NOTE, OWNER_USER_ID, LAYOUT, NFTS) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING ID;`

	layout, err := json.Marshal(pColl.Layout)
	if err != nil {
		return "", err
	}
	var id string
	err = c.db.QueryRowContext(pCtx, sqlStr, persist.GenerateID(), pColl.Version, pColl.Name, pColl.CollectorsNote, pColl.OwnerUserID, string(layout), pq.Array(pColl.Nfts)).Scan(&id)
	if err != nil {
		return "", err
	}
	return persist.DBID(id), nil
}

// GetByUserID returns all collections owned by a user
func (c *CollectionRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID, pShowHidden bool) ([]persist.Collection, error) {
	sqlStr := `SELECT c.ID,array_agg(
		json_build_object(
			  'id',n.ID,'created_at',n.CREATED_AT,'owner_address',n.OWNER_ADDRESS,'multiple_owners',n.MULTIPLE_OWNERS,'name',n.NAME,'contract',n.CONTRACT,'token_collection_name',n.TOKEN_COLLECTION_NAME,'creator_address',n.CREATOR_ADDRESS,'creator_name',n.CREATOR_NAME,'image_url',n.IMAGE_URL,'image_thumnail_url',n.IMAGE_THUMBNAIL_URL,'image_preview_url',n.IMAGE_PREVIEW_URL
			)
		) nfts,c.VERSION,c.DELETED,c.NAME,c.COLLECTORS_NOTE,c.OWNER_USER_ID,c.LAYOUT,c.CREATED_AT,c.LAST_UPDATED 
		FROM collections c JOIN nfts n ON n.ID = ANY (c.NFTS) AND c.OWNER_USER_ID = $1 AND c.HIDDEN = $2 
		GROUP BY c.ID;`
	res, err := c.db.QueryContext(pCtx, sqlStr, pUserID, !pShowHidden)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	var collections []persist.Collection
	for res.Next() {
		var collection persist.Collection
		var nfts []persist.CollectionNFT
		err = res.Scan(&collection.ID, pq.Array(&nfts), &collection.Version, &collection.Deleted, &collection.Name, &collection.CollectorsNote, &collection.OwnerUserID, &collection.Layout, &collection.CreationTime, &collection.LastUpdated)
		if err != nil {
			return nil, err
		}
		collection.Nfts = nfts
		collections = append(collections, collection)
	}
	return collections, nil
}

// GetByID returns a collection by its ID
func (c *CollectionRepository) GetByID(pCtx context.Context, pID persist.DBID, pShowHidden bool) (persist.Collection, error) {

	sqlStr := `SELECT c.ID,array_agg(
		json_build_object(
			  'id',n.ID,'created_at',n.CREATED_AT,'owner_address',n.OWNER_ADDRESS,'multiple_owners',n.MULTIPLE_OWNERS,'name',n.NAME,'contract',n.CONTRACT,'token_collection_name',n.TOKEN_COLLECTION_NAME,'creator_address',n.CREATOR_ADDRESS,'creator_name',n.CREATOR_NAME,'image_url',n.IMAGE_URL,'image_thumnail_url',n.IMAGE_THUMBNAIL_URL,'image_preview_url',n.IMAGE_PREVIEW_URL
			)
		) nfts,c.VERSION,c.DELETED,c.NAME,c.COLLECTORS_NOTE,c.OWNER_USER_ID,c.LAYOUT,c.CREATED_AT,c.LAST_UPDATED 
		FROM collections c JOIN nfts n ON n.ID = ANY (c.NFTS) AND c.ID = $1 AND c.HIDDEN = $2 
		GROUP BY c.ID;`

	var collection persist.Collection
	collection.Nfts = []persist.CollectionNFT{}
	err := c.db.QueryRowContext(pCtx, sqlStr, pID, !pShowHidden).Scan(&collection.ID, pq.Array(&collection.Nfts), &collection.Version, &collection.Deleted, &collection.Name, &collection.CollectorsNote, &collection.OwnerUserID, &collection.Layout, &collection.CreationTime, &collection.LastUpdated)
	if err != nil {
		return persist.Collection{}, err
	}

	return collection, nil
}

// Update updates a collection in the database
func (c *CollectionRepository) Update(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {
	sqlStr := fmt.Sprintf("UPDATE collections %s WHERE ID = $1 AND OWNER_USER_ID = $2;", prepareSet(pUpdate))
	_, err := c.db.ExecContext(pCtx, sqlStr, pID, pUserID)
	return err
}

// UpdateNFTs updates the nfts of a collection in the database
func (c *CollectionRepository) UpdateNFTs(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate persist.CollectionUpdateNftsInput) error {
	sqlStr := `UPDATE collections SET NFTS = $1 WHERE ID = $2 AND OWNER_USER_ID = $3;`
	_, err := c.db.ExecContext(pCtx, sqlStr, pUpdate.Nfts, pID, pUserID)
	return err
}

// ClaimNFTs claims nfts from a collection in the database
func (c *CollectionRepository) ClaimNFTs(pCtx context.Context, pID persist.DBID, pOwnerAddresses []persist.Address, pUpdate persist.CollectionUpdateNftsInput) error {
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
func (c *CollectionRepository) RemoveNFTsOfAddresses(pCtx context.Context, pID persist.DBID, pAddresses []persist.Address) error {
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
func (c *CollectionRepository) Delete(pCtx context.Context, pID persist.DBID, pUserID persist.DBID) error {
	sqlStr := `UPDATE collections SET DELETED = true WHERE ID = $1 AND OWNER_USER_ID = $2;`
	_, err := c.db.ExecContext(pCtx, sqlStr, pID, pUserID)
	return err
}

// GetUnassigned returns all unassigned nfts
func (c *CollectionRepository) GetUnassigned(context.Context, persist.DBID) (persist.Collection, error) {
	return persist.Collection{}, nil
}

// RefreshUnassigned refreshes the unassigned nfts
func (c *CollectionRepository) RefreshUnassigned(context.Context, persist.DBID) error {
	return nil
}
