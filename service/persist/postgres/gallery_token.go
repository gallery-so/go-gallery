package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/persist"
)

// GalleryTokenRepository is the repository for interacting with galleries in a postgres database
type GalleryTokenRepository struct {
	db             *sql.DB
	galleriesCache memstore.Cache
}

// NewGalleryTokenRepository creates a new GalleryTokenRepository
func NewGalleryTokenRepository(db *sql.DB, gCache memstore.Cache) *GalleryTokenRepository {
	return &GalleryTokenRepository{db: db, galleriesCache: gCache}
}

// Create creates a new gallery
func (g *GalleryTokenRepository) Create(pCtx context.Context, pGallery persist.GalleryTokenDB) (persist.DBID, error) {
	sqlStr := `INSERT INTO galleries (ID, VERSION, COLLECTIONS, OWNER_USER_ID) VALUES ($1, $2, $3, $4) RETURNING ID`

	var id persist.DBID
	err := g.db.QueryRowContext(pCtx, sqlStr, persist.GenerateID(), pGallery.Version, pq.Array(pGallery.Collections), pGallery.OwnerUserID).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

// Update updates the gallery with the given ID and ensures that gallery is owned by the given userID
func (g *GalleryTokenRepository) Update(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate persist.GalleryTokenUpdateInput) error {
	sqlStr := `UPDATE galleries SET LAST_UPDATED = $1, COLLECTIONS = $2 WHERE ID = $3 AND OWNER_USER_ID = $4`
	res, err := g.db.ExecContext(pCtx, sqlStr, pUpdate.LastUpdated, pq.Array(pUpdate.Collections), pID, pUserID)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return persist.ErrGalleryNotFoundByID{ID: pID}
	}
	return nil
}

// UpdateUnsafe updates the gallery with the given ID and does not ensure that gallery is owned by the given userID
func (g *GalleryTokenRepository) UpdateUnsafe(pCtx context.Context, pID persist.DBID, pUpdate persist.GalleryTokenUpdateInput) error {
	sqlStr := `UPDATE galleries SET LAST_UPDATED = $1, COLLECTIONS = $2 WHERE ID = $3`
	res, err := g.db.ExecContext(pCtx, sqlStr, pUpdate.LastUpdated, pq.Array(pUpdate.Collections), pID)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return persist.ErrGalleryNotFoundByID{ID: pID}
	}
	return nil
}

// AddCollections adds the given collections to the gallery with the given ID
func (g *GalleryTokenRepository) AddCollections(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pCollections []persist.DBID) error {
	sqlStr := `UPDATE galleries SET COLLECTIONS = COLLECTIONS || $1 WHERE ID = $2 AND OWNER_USER_ID = $3`
	res, err := g.db.ExecContext(pCtx, sqlStr, pq.Array(pCollections), pID, pUserID)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return persist.ErrGalleryNotFoundByID{ID: pID}
	}
	return nil
}

// GetByUserID returns the galleries owned by the given userID
func (g *GalleryTokenRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID) ([]persist.GalleryToken, error) {
	sqlStr := `SELECT g.ID,g.VERSION,g.OWNER_USER_ID,
	c.ID,c.OWNER_USER_ID,c.NAME,c.VERSION,c.DELETED,c.COLLECTORS_NOTE,c.LAYOUT,c.CREATED_AT,c.LAST_UPDATED,
	n.ID,n.OWNER_ADDRESS,n.CHAIN,n.NAME,n.DESCRIPTION,n.TOKEN_TYPE,n.TOKEN_URI,n.TOKEN_ID,n.MEDIA,n.TOKEN_METADATA,n.CONTRACT_ADDRESS,n.CREATED_AT 
	FROM galleries g, unnest(g.COLLECTIONS) WITH ORDINALITY AS u(coll, coll_ord)
	LEFT JOIN collections c ON c.ID = coll 
	LEFT JOIN LATERAL (SELECT n.*,nft,nft_ord FROM tokens n, unnest(c.NFTS) WITH ORDINALITY AS x(nft, nft_ord)) n ON n.ID = n.nft
	WHERE g.OWNER_USER_ID = $1 AND g.DELETED = false ORDER BY coll_ord,n.nft_ord;`
	rows, err := g.db.QueryContext(pCtx, sqlStr, pUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	galleries := make(map[persist.DBID]persist.GalleryToken)
	collections := make(map[persist.DBID][]persist.CollectionToken)
	var gallery persist.GalleryToken
	var collection persist.CollectionToken
	var nft persist.TokenInCollection
	for rows.Next() {
		lastCollID := collection.ID
		err := rows.Scan(&gallery.ID, &gallery.Version, &gallery.OwnerUserID,
			&collection.ID, &collection.OwnerUserID, &collection.Name, &collection.Version, &collection.Deleted, &collection.CollectorsNote,
			&collection.Layout, &collection.CreationTime, &collection.LastUpdated, &nft.ID, &nft.OwnerAddress,
			&nft.Chain, &nft.Name, &nft.Description, &nft.TokenType, &nft.TokenURI, &nft.TokenID, &nft.Media, &nft.TokenMetadata, &nft.ContractAddress, &nft.CreationTime)
		if err != nil {
			return nil, err
		}
		if _, ok := galleries[gallery.ID]; !ok {
			galleries[gallery.ID] = gallery
		}
		colls, ok := collections[gallery.ID]
		if !ok {
			colls = make([]persist.CollectionToken, 0, 10)
			collections[gallery.ID] = colls
		}
		if lastCollID != collection.ID {
			collection.NFTs = []persist.TokenInCollection{nft}
			colls = append(colls, collection)
		} else {
			colls[len(colls)-1].NFTs = append(colls[len(colls)-1].NFTs, nft)
		}
		collections[gallery.ID] = colls
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]persist.GalleryToken, 0, len(galleries))
	for _, gallery := range galleries {
		collections := collections[gallery.ID]
		gallery.Collections = make([]persist.CollectionToken, 0, len(collections))
		for _, coll := range collections {
			gallery.Collections = append(gallery.Collections, coll)
		}
		result = append(result, gallery)
	}
	return result, nil

}

// GetByID returns the gallery with the given ID
func (g *GalleryTokenRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.GalleryToken, error) {
	sqlStr := `SELECT g.ID,g.VERSION,g.OWNER_USER_ID,
	c.ID,c.OWNER_USER_ID,c.NAME,c.VERSION,c.DELETED,c.COLLECTORS_NOTE,c.LAYOUT,c.CREATED_AT,c.LAST_UPDATED,
	n.ID,n.OWNER_ADDRESS,n.CHAIN,n.NAME,n.DESCRIPTION,n.TOKEN_TYPE,n.TOKEN_URI,n.TOKEN_ID,n.MEDIA,n.TOKEN_METADATA,n.CONTRACT_ADDRESS,n.CREATED_AT 
	FROM galleries g, unnest(g.COLLECTIONS) WITH ORDINALITY AS u(coll, coll_ord)
	LEFT JOIN collections c ON c.ID = coll 
	LEFT JOIN LATERAL (SELECT n.*,nft,nft_ord FROM tokens n, unnest(c.NFTS) WITH ORDINALITY AS x(nft, nft_ord)) n ON n.ID = n.nft
	WHERE g.ID = $1 AND g.DELETED = false ORDER BY coll_ord,n.nft_ord;`
	rows, err := g.db.QueryContext(pCtx, sqlStr, pID)
	if err != nil {
		return persist.GalleryToken{}, err
	}
	defer rows.Close()

	galleries := make(map[persist.DBID]persist.GalleryToken)
	collections := make(map[persist.DBID][]persist.CollectionToken)
	var gallery persist.GalleryToken
	var collection persist.CollectionToken
	var nft persist.TokenInCollection
	for rows.Next() {
		lastCollID := collection.ID

		err := rows.Scan(&gallery.ID, &gallery.Version, &gallery.OwnerUserID,
			&collection.ID, &collection.OwnerUserID, &collection.Name, &collection.Version, &collection.Deleted, &collection.CollectorsNote,
			&collection.Layout, &collection.CreationTime, &collection.LastUpdated, &nft.ID, &nft.OwnerAddress,
			&nft.Chain, &nft.Name, &nft.Description, &nft.TokenType, &nft.TokenURI, &nft.TokenID, &nft.Media, &nft.TokenMetadata, &nft.ContractAddress, &nft.CreationTime)
		if err != nil {
			return persist.GalleryToken{}, err
		}
		if _, ok := galleries[gallery.ID]; !ok {
			galleries[gallery.ID] = gallery
		}
		colls, ok := collections[gallery.ID]
		if !ok {
			colls = make([]persist.CollectionToken, 0, 10)
			collections[gallery.ID] = colls
		}
		if lastCollID != collection.ID {
			collection.NFTs = []persist.TokenInCollection{nft}
			colls = append(colls, collection)
		} else {
			colls[len(colls)-1].NFTs = append(colls[len(colls)-1].NFTs, nft)
		}
		collections[gallery.ID] = colls
	}
	if err := rows.Err(); err != nil {
		return persist.GalleryToken{}, err
	}

	if len(galleries) > 1 {
		return persist.GalleryToken{}, errors.New("too many galleries")
	}

	for _, gallery := range galleries {
		collections := collections[gallery.ID]
		gallery.Collections = make([]persist.CollectionToken, 0, len(collections))
		for _, coll := range collections {
			gallery.Collections = append(gallery.Collections, coll)
		}
		return gallery, nil
	}
	return persist.GalleryToken{}, errors.New("no gallery found")
}

// RefreshCache deletes the given key in the cache
func (g *GalleryTokenRepository) RefreshCache(pCtx context.Context, pUserID persist.DBID) error {
	return g.galleriesCache.Delete(pCtx, pUserID.String())
}
