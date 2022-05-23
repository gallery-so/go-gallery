package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/mikeydub/go-gallery/service/logger"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/persist"
)

var errCollsNotOwnedByUser = errors.New("collections not owned by user")

// GalleryTokenRepository is the repository for interacting with galleries in a postgres database
type GalleryTokenRepository struct {
	db                        *sql.DB
	createStmt                *sql.Stmt
	updateStmt                *sql.Stmt
	updateUnsafeStmt          *sql.Stmt
	addCollectionsStmt        *sql.Stmt
	getByUserIDStmt           *sql.Stmt
	getByIDStmt               *sql.Stmt
	getByUserIDRawStmt        *sql.Stmt
	getByIDRawStmt            *sql.Stmt
	checkOwnCollectionsStmt   *sql.Stmt
	countAllCollectionsStmt   *sql.Stmt
	countCollsStmt            *sql.Stmt
	getCollectionsStmt        *sql.Stmt
	getGalleryCollectionsStmt *sql.Stmt

	galleriesCache memstore.Cache
}

// NewGalleryTokenRepository creates a new GalleryTokenRepository
// TODO another join to addresses
func NewGalleryTokenRepository(db *sql.DB, gCache memstore.Cache) *GalleryTokenRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx, `INSERT INTO galleries (ID, VERSION, COLLECTIONS, OWNER_USER_ID) VALUES ($1, $2, $3, $4) RETURNING ID;`)
	checkNoErr(err)

	updateStmt, err := db.PrepareContext(ctx, `UPDATE galleries SET LAST_UPDATED = $1, COLLECTIONS = $2 WHERE ID = $3 AND OWNER_USER_ID = $4;`)
	checkNoErr(err)

	updateUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE galleries SET LAST_UPDATED = $1, COLLECTIONS = $2 WHERE ID = $3;`)
	checkNoErr(err)

	addCollectionsStmt, err := db.PrepareContext(ctx, `UPDATE galleries SET COLLECTIONS = $1 || COLLECTIONS WHERE ID = $2 AND OWNER_USER_ID = $3;`)
	checkNoErr(err)

	getByUserIDStmt, err := db.PrepareContext(ctx, `SELECT g.ID,g.VERSION,g.OWNER_USER_ID,g.CREATED_AT,g.LAST_UPDATED,
	c.ID,c.OWNER_USER_ID,c.NAME,c.VERSION,c.DELETED,c.COLLECTORS_NOTE,c.LAYOUT,c.HIDDEN,c.CREATED_AT,c.LAST_UPDATED,
	n.ID,n.OWNER_USER_ID,n.CHAIN,n.NAME,n.DESCRIPTION,n.TOKEN_TYPE,n.TOKEN_URI,n.TOKEN_ID,n.MEDIA,n.TOKEN_METADATA,n.CONTRACT_ADDRESS,n.CREATED_AT 
	FROM galleries g, unnest(g.COLLECTIONS) WITH ORDINALITY AS u(coll, coll_ord)
	LEFT JOIN collections_v2 c ON c.ID = coll AND c.DELETED = false
	LEFT JOIN LATERAL (SELECT n.*,nft,nft_ord FROM tokens n, unnest(c.NFTS) WITH ORDINALITY AS x(nft, nft_ord)) n ON n.ID = n.nft
	WHERE g.OWNER_USER_ID = $1 AND g.DELETED = false ORDER BY coll_ord,n.nft_ord;`)
	checkNoErr(err)

	getByIDStmt, err := db.PrepareContext(ctx, `SELECT g.ID,g.VERSION,g.OWNER_USER_ID,g.CREATED_AT,g.LAST_UPDATED,
	c.ID,c.OWNER_USER_ID,c.NAME,c.VERSION,c.DELETED,c.COLLECTORS_NOTE,c.LAYOUT,c.HIDDEN,c.CREATED_AT,c.LAST_UPDATED,
	n.ID,n.OWNER_USER_ID,n.CHAIN,n.NAME,n.DESCRIPTION,n.TOKEN_TYPE,n.TOKEN_URI,n.TOKEN_ID,n.MEDIA,n.TOKEN_METADATA,n.CONTRACT_ADDRESS,n.CREATED_AT
	FROM galleries g, unnest(g.COLLECTIONS) WITH ORDINALITY AS u(coll, coll_ord)
	LEFT JOIN collections_v2 c ON c.ID = coll AND c.DELETED = false
	LEFT JOIN LATERAL (SELECT n.*,nft,nft_ord FROM tokens n, unnest(c.NFTS) WITH ORDINALITY AS x(nft, nft_ord)) n ON n.ID = n.nft
	WHERE g.ID = $1 AND g.DELETED = false ORDER BY coll_ord,n.nft_ord;`)
	checkNoErr(err)

	getByUserIDRawStmt, err := db.PrepareContext(ctx, `SELECT g.ID,g.VERSION,g.OWNER_USER_ID,g.CREATED_AT,g.LAST_UPDATED FROM galleries g WHERE g.OWNER_USER_ID = $1 AND g.DELETED = false;`)
	checkNoErr(err)

	getByIDRawStmt, err := db.PrepareContext(ctx, `SELECT g.ID,g.VERSION,g.OWNER_USER_ID,g.CREATED_AT,g.LAST_UPDATED FROM galleries g WHERE g.ID = $1 AND g.DELETED = false;`)
	checkNoErr(err)

	checkOwnCollectionsStmt, err := db.PrepareContext(ctx, `SELECT COUNT(*) FROM collections_v2 WHERE ID = ANY($1) AND OWNER_USER_ID = $2;`)
	checkNoErr(err)

	countAllCollectionsStmt, err := db.PrepareContext(ctx, `SELECT COUNT(*) FROM collections_v2 WHERE OWNER_USER_ID = $1 AND DELETED = false;`)
	checkNoErr(err)

	countCollsStmt, err := db.PrepareContext(ctx, `SELECT COUNT(c.ID) FROM galleries g, unnest(g.COLLECTIONS) WITH ORDINALITY AS u(coll, coll_ord) LEFT JOIN collections_v2 c ON c.ID = coll WHERE g.ID = $1 AND c.DELETED = false and g.DELETED = false;`)
	checkNoErr(err)

	getCollectionsStmt, err := db.PrepareContext(ctx, `SELECT ID FROM collections_v2 WHERE OWNER_USER_ID = $1 AND DELETED = false;`)
	checkNoErr(err)

	getGalleryCollectionsStmt, err := db.PrepareContext(ctx, `SELECT array_agg(c.ID) FROM galleries g, unnest(g.COLLECTIONS) WITH ORDINALITY AS u(coll, coll_ord) LEFT JOIN collections_v2 c ON c.ID = coll WHERE g.ID = $1 AND c.DELETED = false and g.DELETED = false GROUP BY coll_ord ORDER BY coll_ord;`)
	checkNoErr(err)

	return &GalleryTokenRepository{db: db, createStmt: createStmt, updateStmt: updateStmt, updateUnsafeStmt: updateUnsafeStmt, addCollectionsStmt: addCollectionsStmt, getByUserIDStmt: getByUserIDStmt, getByIDStmt: getByIDStmt, galleriesCache: gCache, checkOwnCollectionsStmt: checkOwnCollectionsStmt, countAllCollectionsStmt: countAllCollectionsStmt, countCollsStmt: countCollsStmt, getCollectionsStmt: getCollectionsStmt, getGalleryCollectionsStmt: getGalleryCollectionsStmt, getByUserIDRawStmt: getByUserIDRawStmt, getByIDRawStmt: getByIDRawStmt}
}

// Create creates a new gallery
func (g *GalleryTokenRepository) Create(pCtx context.Context, pGallery persist.GalleryTokenDB) (persist.DBID, error) {

	err := ensureCollsOwnedByUserToken(pCtx, g, pGallery.Collections, pGallery.OwnerUserID)
	if err != nil {
		return "", err
	}

	colls, err := ensureAllCollsAccountedForToken(pCtx, g, pGallery.Collections, pGallery.OwnerUserID)
	if err != nil {
		return "", err
	}
	pGallery.Collections = colls

	var id persist.DBID
	err = g.createStmt.QueryRowContext(pCtx, persist.GenerateID(), pGallery.Version, pq.Array(pGallery.Collections), pGallery.OwnerUserID).Scan(&id)
	if err != nil {
		return "", err
	}
	err = g.cacheByUserID(pCtx, pGallery.OwnerUserID)
	if err != nil {
		return "", err
	}

	return id, nil
}

// Update updates the gallery with the given ID and ensures that gallery is owned by the given userID
func (g *GalleryTokenRepository) Update(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate persist.GalleryTokenUpdateInput) error {
	err := ensureCollsOwnedByUserToken(pCtx, g, pUpdate.Collections, pUserID)
	if err != nil {
		return err
	}
	colls, err := ensureAllCollsAccountedForToken(pCtx, g, pUpdate.Collections, pUserID)
	if err != nil {
		return err
	}
	pUpdate.Collections = colls
	res, err := g.updateStmt.ExecContext(pCtx, pUpdate.LastUpdated, pq.Array(pUpdate.Collections), pID, pUserID)
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
	err = g.cacheByUserID(pCtx, pUserID)
	if err != nil {
		return err
	}

	return nil
}

// UpdateUnsafe updates the gallery with the given ID and does not ensure that gallery is owned by the given userID
func (g *GalleryTokenRepository) UpdateUnsafe(pCtx context.Context, pID persist.DBID, pUpdate persist.GalleryTokenUpdateInput) error {
	res, err := g.updateUnsafeStmt.ExecContext(pCtx, pUpdate.LastUpdated, pq.Array(pUpdate.Collections), pID)
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
	err = g.cacheByID(pCtx, pID)
	if err != nil {
		return err
	}
	return nil
}

// AddCollections adds the given collections to the gallery with the given ID
func (g *GalleryTokenRepository) AddCollections(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pCollections []persist.DBID) error {

	err := ensureCollsOwnedByUserToken(pCtx, g, pCollections, pUserID)
	if err != nil {
		return err
	}

	var ct persist.NullInt64
	err = g.countCollsStmt.QueryRowContext(pCtx, pID).Scan(&ct)
	if err != nil {
		return err
	}

	var allCollsCt int64
	err = g.countAllCollectionsStmt.QueryRowContext(pCtx, pUserID).Scan(&allCollsCt)
	if err != nil {
		return err
	}

	if ct.Int64()+int64(len(pCollections)) != allCollsCt {

		var galleryCollIDs []persist.DBID
		err = g.getGalleryCollectionsStmt.QueryRowContext(pCtx, pID).Scan(pq.Array(&galleryCollIDs))
		if err != nil {
			return err
		}
		galleryCollIDs = append(pCollections, galleryCollIDs...)

		allColls, err := addUnaccountedForCollectionsToken(pCtx, g, pUserID, galleryCollIDs)
		if err != nil {
			return err
		}
		res, err := g.updateStmt.ExecContext(pCtx, time.Now(), pq.Array(allColls), pID, pUserID)
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

	res, err := g.addCollectionsStmt.ExecContext(pCtx, pq.Array(pCollections), pID, pUserID)
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

	err = g.cacheByUserID(pCtx, pUserID)
	if err != nil {
		return err
	}
	return nil
}

// GetByUserID returns the galleries owned by the given userID
func (g *GalleryTokenRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID) ([]persist.GalleryToken, error) {
	if g.galleriesCache != nil {
		initial, _ := g.galleriesCache.Get(pCtx, pUserID.String())
		if len(initial) > 0 {
			var galleries []persist.GalleryToken
			err := json.Unmarshal(initial, &galleries)
			if err != nil {
				logger.For(pCtx).WithError(err).Errorf("failed to unmarshal galleries cache for userID %s - cached: %s", pUserID, string(initial))
			} else {
				return galleries, nil
			}
		}
	}
	rows, err := g.getByUserIDStmt.QueryContext(pCtx, pUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	galleries := make(map[persist.DBID]persist.GalleryToken)
	collections := make(map[persist.DBID][]persist.CollectionToken)
	var gallery persist.GalleryToken
	var lastCollID persist.DBID
	for rows.Next() {
		var collection persist.CollectionToken
		var nft persist.TokenInCollection

		err := rows.Scan(&gallery.ID, &gallery.Version, &gallery.OwnerUserID, &gallery.CreationTime, &gallery.LastUpdated,
			&collection.ID, &collection.OwnerUserID, &collection.Name, &collection.Version, &collection.Deleted, &collection.CollectorsNote,
			&collection.Layout, &collection.Hidden, &collection.CreationTime, &collection.LastUpdated, &nft.ID, &nft.OwnerAddress,
			&nft.Chain, &nft.Name, &nft.Description, &nft.TokenType, &nft.TokenURI, &nft.TokenID, &nft.Media, &nft.TokenMetadata, &nft.ContractAddress, &nft.CreationTime)
		if err != nil {
			return nil, err
		}
		if _, ok := galleries[gallery.ID]; !ok {
			galleries[gallery.ID] = gallery
		}

		if collection.ID == "" {
			continue
		}
		colls, ok := collections[gallery.ID]
		if !ok {
			colls = make([]persist.CollectionToken, 0, 10)

		}
		if lastCollID != collection.ID {
			if nft.ID != "" {
				collection.NFTs = []persist.TokenInCollection{nft}
			} else {
				collection.NFTs = []persist.TokenInCollection{}
			}
			colls = append(colls, collection)
		} else {
			lastColl := colls[len(colls)-1]
			lastColl.NFTs = append(lastColl.NFTs, nft)
			colls[len(colls)-1] = lastColl
		}

		collections[gallery.ID] = colls
		lastCollID = collection.ID
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]persist.GalleryToken, 0, len(galleries))

	if len(galleries) == 0 {
		galleriesRaw, err := g.getByUserIDRawStmt.QueryContext(pCtx, pUserID)
		if err != nil {
			return nil, err
		}
		defer galleriesRaw.Close()
		for galleriesRaw.Next() {
			var rawGallery persist.GalleryToken
			err := galleriesRaw.Scan(&rawGallery.ID, &rawGallery.Version, &rawGallery.OwnerUserID, &rawGallery.CreationTime, &rawGallery.LastUpdated)
			if err != nil {
				return nil, err
			}
			rawGallery.Collections = []persist.CollectionToken{}
			result = append(result, rawGallery)
		}
		if err := galleriesRaw.Err(); err != nil {
			return nil, err
		}
		return result, nil
	}

	for _, gallery := range galleries {
		collections := collections[gallery.ID]
		gallery.Collections = make([]persist.CollectionToken, 0, len(collections))
		for _, coll := range collections {
			if coll.ID == "" {
				continue
			}
			gallery.Collections = append(gallery.Collections, coll)
		}
		result = append(result, gallery)
	}

	if g.galleriesCache != nil {
		marshalled, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		if err := g.galleriesCache.Set(pCtx, pUserID.String(), marshalled, galleryCacheTime); err != nil {
			return nil, err
		}
	}
	return result, nil

}

// GetByID returns the gallery with the given ID
func (g *GalleryTokenRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.GalleryToken, error) {
	rows, err := g.getByIDStmt.QueryContext(pCtx, pID)
	if err != nil {
		return persist.GalleryToken{}, err
	}
	defer rows.Close()

	galleries := make(map[persist.DBID]persist.GalleryToken)
	collections := make(map[persist.DBID][]persist.CollectionToken)
	var gallery persist.GalleryToken
	var lastCollID persist.DBID
	for rows.Next() {
		var collection persist.CollectionToken
		var nft persist.TokenInCollection

		err := rows.Scan(&gallery.ID, &gallery.Version, &gallery.OwnerUserID, &gallery.CreationTime, &gallery.LastUpdated,
			&collection.ID, &collection.OwnerUserID, &collection.Name, &collection.Version, &collection.Deleted, &collection.CollectorsNote,
			&collection.Layout, &collection.Hidden, &collection.CreationTime, &collection.LastUpdated, &nft.ID, &nft.OwnerAddress,
			&nft.Chain, &nft.Name, &nft.Description, &nft.TokenType, &nft.TokenURI, &nft.TokenID, &nft.Media, &nft.TokenMetadata, &nft.ContractAddress, &nft.CreationTime)
		if err != nil {
			return persist.GalleryToken{}, err
		}
		if _, ok := galleries[gallery.ID]; !ok {
			galleries[gallery.ID] = gallery
		}

		if collection.ID == "" {
			continue
		}
		colls, ok := collections[gallery.ID]
		if !ok {
			colls = make([]persist.CollectionToken, 0, 10)

		}
		if lastCollID != collection.ID {
			if nft.ID != "" {
				collection.NFTs = []persist.TokenInCollection{nft}
			} else {
				collection.NFTs = []persist.TokenInCollection{}
			}
			colls = append(colls, collection)
		} else {
			lastColl := colls[len(colls)-1]
			lastColl.NFTs = append(lastColl.NFTs, nft)
			colls[len(colls)-1] = lastColl

		}

		collections[gallery.ID] = colls
		lastCollID = collection.ID
	}
	if err := rows.Err(); err != nil {
		return persist.GalleryToken{}, err
	}

	if len(galleries) > 1 {
		return persist.GalleryToken{}, errors.New("too many galleries")
	}

	if len(galleries) == 0 {
		res := persist.GalleryToken{Collections: []persist.CollectionToken{}}
		err := g.getByUserIDRawStmt.QueryRowContext(pCtx, pID).Scan(&res.ID, &res.Version, &res.OwnerUserID, &res.CreationTime, &res.LastUpdated)
		if err != nil {
			return persist.GalleryToken{}, err
		}
		if res.ID != pID {
			return persist.GalleryToken{}, persist.ErrGalleryNotFoundByID{ID: pID}
		}
		return res, nil
	}

	for _, gallery := range galleries {
		collections := collections[gallery.ID]
		gallery.Collections = make([]persist.CollectionToken, 0, len(collections))
		for _, coll := range collections {
			if coll.ID == "" {
				continue
			}
			gallery.Collections = append(gallery.Collections, coll)
		}
		return gallery, nil
	}
	return persist.GalleryToken{}, persist.ErrGalleryNotFoundByID{ID: pID}
}

// RefreshCache deletes the given key in the cache
func (g *GalleryTokenRepository) RefreshCache(pCtx context.Context, pUserID persist.DBID) error {
	return g.galleriesCache.Delete(pCtx, pUserID.String())
}

func ensureCollsOwnedByUserToken(pCtx context.Context, g *GalleryTokenRepository, pColls []persist.DBID, pUserID persist.DBID) error {
	var ct int64
	err := g.checkOwnCollectionsStmt.QueryRowContext(pCtx, pq.Array(pColls), pUserID).Scan(&ct)
	if err != nil {
		return err
	}
	if ct != int64(len(pColls)) {
		return errCollsNotOwnedByUser
	}
	return nil
}

func ensureAllCollsAccountedForToken(pCtx context.Context, g *GalleryTokenRepository, pColls []persist.DBID, pUserID persist.DBID) ([]persist.DBID, error) {
	var ct int64
	err := g.countAllCollectionsStmt.QueryRowContext(pCtx, pUserID).Scan(&ct)
	if err != nil {
		return nil, err
	}
	if ct != int64(len(pColls)) {
		if int64(len(pColls)) < ct {
			return addUnaccountedForCollectionsToken(pCtx, g, pUserID, pColls)
		}
		return nil, errCollsNotOwnedByUser
	}
	return pColls, nil
}

func appendDifference(pDest []persist.DBID, pSrc []persist.DBID) []persist.DBID {
	for _, v := range pSrc {
		if !persist.ContainsDBID(pDest, v) {
			pDest = append(pDest, v)
		}
	}
	return pDest
}

func addUnaccountedForCollectionsToken(pCtx context.Context, g *GalleryTokenRepository, pUserID persist.DBID, pColls []persist.DBID) ([]persist.DBID, error) {
	rows, err := g.getCollectionsStmt.QueryContext(pCtx, pUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	colls := make([]persist.DBID, 0, len(pColls)*2)
	for rows.Next() {
		var coll persist.DBID
		if err := rows.Scan(&coll); err != nil {
			return nil, err
		}
		colls = append(colls, coll)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return appendDifference(pColls, colls), nil
}

func (g *GalleryTokenRepository) cacheByID(pCtx context.Context, pID persist.DBID) error {
	gal, err := g.GetByID(pCtx, pID)
	if err != nil {
		return err
	}
	gals, err := g.GetByUserID(pCtx, gal.OwnerUserID)
	if err != nil {
		return err
	}
	marshalled, err := json.Marshal(gals)
	if err != nil {
		return err
	}
	if err = g.galleriesCache.Set(pCtx, gal.OwnerUserID.String(), marshalled, -1); err != nil {
		return err
	}
	return nil
}

func (g *GalleryTokenRepository) cacheByUserID(pCtx context.Context, pUserID persist.DBID) error {
	err := g.RefreshCache(pCtx, pUserID)
	if err != nil {
		return err
	}
	gals, err := g.GetByUserID(pCtx, pUserID)
	if err != nil {
		return err
	}
	marshalled, err := json.Marshal(gals)
	if err != nil {
		return err
	}
	if err = g.galleriesCache.Set(pCtx, pUserID.String(), marshalled, galleryCacheTime); err != nil {
		return err
	}
	return nil
}
