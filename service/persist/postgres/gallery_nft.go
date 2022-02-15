package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
)

// GalleryRepository is the repository for interacting with galleries in a postgres database
type GalleryRepository struct {
	db                        *sql.DB
	createStmt                *sql.Stmt
	updateStmt                *sql.Stmt
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

// NewGalleryRepository creates a new GalleryRepository
func NewGalleryRepository(db *sql.DB, gCache memstore.Cache) *GalleryRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx, `INSERT INTO galleries (ID, VERSION, COLLECTIONS, OWNER_USER_ID) VALUES ($1, $2, $3, $4) RETURNING ID;`)
	checkNoErr(err)

	updateStmt, err := db.PrepareContext(ctx, `UPDATE galleries SET LAST_UPDATED = $1, COLLECTIONS = $2 WHERE ID = $3 AND OWNER_USER_ID = $4;`)
	checkNoErr(err)

	addCollectionsStmt, err := db.PrepareContext(ctx, `UPDATE galleries SET COLLECTIONS = COLLECTIONS || $1 WHERE ID = $2 AND OWNER_USER_ID = $3;`)
	checkNoErr(err)

	getByUserIDStmt, err := db.PrepareContext(ctx, `SELECT g.ID,g.VERSION,g.OWNER_USER_ID,g.CREATED_AT,g.LAST_UPDATED,
	c.ID,c.OWNER_USER_ID,c.NAME,c.VERSION,c.DELETED,c.COLLECTORS_NOTE,
	c.LAYOUT,c.HIDDEN,c.CREATED_AT,c.LAST_UPDATED,
	n.ID,n.OWNER_ADDRESS,
	n.MULTIPLE_OWNERS,n.NAME,n.CONTRACT,n.TOKEN_COLLECTION_NAME,n.CREATOR_ADDRESS,n.CREATOR_NAME,
	n.IMAGE_URL,n.IMAGE_THUMBNAIL_URL,n.IMAGE_PREVIEW_URL,n.ANIMATION_ORIGINAL_URL,n.ANIMATION_URL,n.CREATED_AT 
	FROM galleries g, unnest(g.COLLECTIONS) WITH ORDINALITY AS u(coll, coll_ord)
	LEFT JOIN collections c ON c.ID = coll AND c.DELETED = false
	LEFT JOIN LATERAL (SELECT n.*,nft,nft_ord FROM nfts n, unnest(c.NFTS) WITH ORDINALITY AS x(nft, nft_ord)) n ON n.ID = n.nft
	WHERE g.OWNER_USER_ID = $1 AND g.DELETED = false ORDER BY coll_ord,n.nft_ord;`)
	checkNoErr(err)

	getByIDStmt, err := db.PrepareContext(ctx, `SELECT g.ID,g.VERSION,g.OWNER_USER_ID,g.CREATED_AT,g.LAST_UPDATED,
	c.ID,c.OWNER_USER_ID,c.NAME,c.VERSION,c.DELETED,c.COLLECTORS_NOTE,
	c.LAYOUT,c.HIDDEN,c.CREATED_AT,c.LAST_UPDATED,n.ID,n.OWNER_ADDRESS,
	n.MULTIPLE_OWNERS,n.NAME,n.CONTRACT,n.TOKEN_COLLECTION_NAME,n.CREATOR_ADDRESS,n.CREATOR_NAME, 
	n.IMAGE_URL,n.IMAGE_THUMBNAIL_URL,n.IMAGE_PREVIEW_URL,n.ANIMATION_ORIGINAL_URL,n.ANIMATION_URL,n.CREATED_AT 
	FROM galleries g, unnest(g.COLLECTIONS) WITH ORDINALITY AS u(coll, coll_ord)
	LEFT JOIN collections c ON c.ID = coll AND c.DELETED = false
	LEFT JOIN LATERAL (SELECT n.*,nft,nft_ord FROM nfts n, unnest(c.NFTS) WITH ORDINALITY AS x(nft, nft_ord)) n ON n.ID = n.nft
	WHERE g.ID = $1 AND g.DELETED = false ORDER BY coll_ord,n.nft_ord;`)
	checkNoErr(err)

	getByUserIDRawStmt, err := db.PrepareContext(ctx, `SELECT g.ID,g.VERSION,g.OWNER_USER_ID,g.CREATED_AT,g.LAST_UPDATED FROM galleries g WHERE g.OWNER_USER_ID = $1 AND g.DELETED = false;`)
	checkNoErr(err)

	getByIDRawStmt, err := db.PrepareContext(ctx, `SELECT g.ID,g.VERSION,g.OWNER_USER_ID,g.CREATED_AT,g.LAST_UPDATED FROM galleries g WHERE g.ID = $1 AND g.DELETED = false;`)
	checkNoErr(err)

	checkOwnCollectionsStmt, err := db.PrepareContext(ctx, `SELECT COUNT(*) FROM collections WHERE ID = ANY($1) AND OWNER_USER_ID = $2;`)
	checkNoErr(err)

	countAllCollectionsStmt, err := db.PrepareContext(ctx, `SELECT COUNT(*) FROM collections WHERE OWNER_USER_ID = $1 AND DELETED = false;`)
	checkNoErr(err)

	countCollsStmt, err := db.PrepareContext(ctx, `SELECT COUNT(c.ID) FROM galleries g, unnest(g.COLLECTIONS) WITH ORDINALITY AS u(coll, coll_ord) LEFT JOIN collections c ON c.ID = coll WHERE g.ID = $1 AND c.DELETED = false and g.DELETED = false;`)
	checkNoErr(err)

	getCollectionsStmt, err := db.PrepareContext(ctx, `SELECT ID FROM collections WHERE OWNER_USER_ID = $1 AND DELETED = false;`)
	checkNoErr(err)

	getGalleryCollectionsStmt, err := db.PrepareContext(ctx, `SELECT array_agg(c.ID) FROM galleries g, unnest(g.COLLECTIONS) WITH ORDINALITY AS u(coll, coll_ord) LEFT JOIN collections c ON c.ID = coll WHERE g.ID = $1 AND c.DELETED = false and g.DELETED = false GROUP BY coll_ord ORDER BY coll_ord;`)
	checkNoErr(err)

	return &GalleryRepository{db: db, createStmt: createStmt, updateStmt: updateStmt, addCollectionsStmt: addCollectionsStmt, getByUserIDStmt: getByUserIDStmt, getByIDStmt: getByIDStmt, galleriesCache: gCache, checkOwnCollectionsStmt: checkOwnCollectionsStmt, countAllCollectionsStmt: countAllCollectionsStmt, countCollsStmt: countCollsStmt, getCollectionsStmt: getCollectionsStmt, getGalleryCollectionsStmt: getGalleryCollectionsStmt, getByUserIDRawStmt: getByUserIDRawStmt, getByIDRawStmt: getByIDRawStmt}
}

// Create creates a new gallery
func (g *GalleryRepository) Create(pCtx context.Context, pGallery persist.GalleryDB) (persist.DBID, error) {

	err := ensureCollsOwnedByUser(pCtx, g, pGallery.Collections, pGallery.OwnerUserID)
	if err != nil {
		return "", err
	}
	colls, err := ensureAllCollsAccountedFor(pCtx, g, pGallery.Collections, pGallery.OwnerUserID)
	if err != nil {
		return "", err
	}
	pGallery.Collections = colls

	var id persist.DBID
	err = g.createStmt.QueryRowContext(pCtx, persist.GenerateID(), pGallery.Version, pq.Array(pGallery.Collections), pGallery.OwnerUserID).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

// Update updates the gallery with the given ID and ensures that gallery is owned by the given userID
func (g *GalleryRepository) Update(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate persist.GalleryUpdateInput) error {
	err := ensureCollsOwnedByUser(pCtx, g, pUpdate.Collections, pUserID)
	if err != nil {
		return err
	}
	colls, err := ensureAllCollsAccountedFor(pCtx, g, pUpdate.Collections, pUserID)
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
	return nil
}

// AddCollections adds the given collections to the gallery with the given ID
func (g *GalleryRepository) AddCollections(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pCollections []persist.DBID) error {
	err := ensureCollsOwnedByUser(pCtx, g, pCollections, pUserID)
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
		galleryCollIDs = append(galleryCollIDs, pCollections...)

		allColls, err := addUnaccountedForCollections(pCtx, g, pUserID, galleryCollIDs)
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
	return nil
}

// GetByUserID returns the galleries owned by the given userID
func (g *GalleryRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID) ([]persist.Gallery, error) {
	rows, err := g.getByUserIDStmt.QueryContext(pCtx, pUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	galleries := make(map[persist.DBID]persist.Gallery)
	collections := make(map[persist.DBID][]persist.Collection)
	var gallery persist.Gallery
	var collection persist.Collection
	for rows.Next() {
		lastCollID := collection.ID
		var nft persist.CollectionNFT

		err := rows.Scan(&gallery.ID, &gallery.Version, &gallery.OwnerUserID, &gallery.CreationTime, &gallery.LastUpdated,
			&collection.ID, &collection.OwnerUserID, &collection.Name, &collection.Version, &collection.Deleted, &collection.CollectorsNote,
			&collection.Layout, &collection.Hidden, &collection.CreationTime, &collection.LastUpdated, &nft.ID, &nft.OwnerAddress,
			&nft.MultipleOwners, &nft.Name, &nft.Contract, &nft.TokenCollectionName, &nft.CreatorAddress, &nft.CreatorName,
			&nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.AnimationOriginalURL, &nft.AnimationURL, &nft.CreationTime)
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
			logrus.Debugf("First time seeing collections for gallery %s", gallery.ID)
			colls = make([]persist.Collection, 0, 10)
		}

		if lastCollID != collection.ID {
			logrus.Debugf("Adding collection %s to gallery %s", collection.ID, gallery.ID)
			if nft.ID != "" {
				logrus.Debugf("Adding NFT %s to collection %s", nft.ID, collection.ID)
				collection.NFTs = []persist.CollectionNFT{nft}
			} else {
				collection.NFTs = []persist.CollectionNFT{}
				logrus.Debugf("No NFTs found for collection %s", collection.ID)
			}
			colls = append(colls, collection)
		} else {
			logrus.Debugf("Already seen: Adding NFT %s to collection at end of current colls len %d", nft.ID, len(colls))
			lastColl := colls[len(colls)-1]
			lastColl.NFTs = append(lastColl.NFTs, nft)
			colls[len(colls)-1] = lastColl
		}

		collections[gallery.ID] = colls
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]persist.Gallery, 0, len(galleries))

	if len(galleries) == 0 {
		galleriesRaw, err := g.getByUserIDRawStmt.QueryContext(pCtx, pUserID)
		if err != nil {
			return nil, err
		}
		defer galleriesRaw.Close()
		for galleriesRaw.Next() {
			var rawGallery persist.Gallery
			err := galleriesRaw.Scan(&rawGallery.ID, &rawGallery.Version, &rawGallery.OwnerUserID, &rawGallery.CreationTime, &rawGallery.LastUpdated)
			if err != nil {
				return nil, err
			}
			rawGallery.Collections = []persist.Collection{}
			result = append(result, rawGallery)
		}
		if err := galleriesRaw.Err(); err != nil {
			return nil, err
		}
		return result, nil
	}

	for _, gallery := range galleries {
		collections := collections[gallery.ID]
		gallery.Collections = make([]persist.Collection, 0, len(collections))
		for _, coll := range collections {
			if coll.ID == "" {
				continue
			}
			gallery.Collections = append(gallery.Collections, coll)
		}
		result = append(result, gallery)
	}
	return result, nil

}

// GetByID returns the gallery with the given ID
func (g *GalleryRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.Gallery, error) {
	rows, err := g.getByIDStmt.QueryContext(pCtx, pID)
	if err != nil {
		return persist.Gallery{}, err
	}
	defer rows.Close()

	galleries := make(map[persist.DBID]persist.Gallery)
	collections := make(map[persist.DBID][]persist.Collection)
	var gallery persist.Gallery
	var collection persist.Collection

	for rows.Next() {
		var nft persist.CollectionNFT
		lastCollID := collection.ID

		err := rows.Scan(&gallery.ID, &gallery.Version, &gallery.OwnerUserID, &gallery.CreationTime, &gallery.LastUpdated,
			&collection.ID, &collection.OwnerUserID, &collection.Name, &collection.Version, &collection.Deleted, &collection.CollectorsNote,
			&collection.Layout, &collection.Hidden, &collection.CreationTime, &collection.LastUpdated, &nft.ID, &nft.OwnerAddress,
			&nft.MultipleOwners, &nft.Name, &nft.Contract, &nft.TokenCollectionName, &nft.CreatorAddress, &nft.CreatorName,
			&nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.AnimationOriginalURL, &nft.AnimationURL, &nft.CreationTime)
		if err != nil {
			return persist.Gallery{}, err
		}
		if _, ok := galleries[gallery.ID]; !ok {
			galleries[gallery.ID] = gallery
		}

		if collection.ID == "" {
			continue
		}

		colls, ok := collections[gallery.ID]
		if !ok {
			logrus.Debugf("First time seeing collections for gallery %s", gallery.ID)
			colls = make([]persist.Collection, 0, 10)
		}

		if lastCollID != collection.ID {
			logrus.Infof("Adding collection %s to gallery %s", collection.ID, gallery.ID)
			if nft.ID != "" {
				logrus.Infof("Adding NFT %s to collection %s", nft.ID, collection.ID)
				collection.NFTs = []persist.CollectionNFT{nft}
			} else {
				collection.NFTs = []persist.CollectionNFT{}
				logrus.Infof("No NFTs found for collection %s", collection.ID)
			}
			colls = append(colls, collection)
		} else {
			lastColl := colls[len(colls)-1]
			lastColl.NFTs = append(lastColl.NFTs, nft)
			colls[len(colls)-1] = lastColl
		}

		collections[gallery.ID] = colls
	}
	if err := rows.Err(); err != nil {
		return persist.Gallery{}, err
	}

	if len(galleries) > 1 {
		return persist.Gallery{}, errors.New("too many galleries")
	}

	if len(galleries) == 0 {
		res := persist.Gallery{Collections: []persist.Collection{}}
		err := g.getByUserIDRawStmt.QueryRowContext(pCtx, pID).Scan(&res.ID, &res.Version, &res.OwnerUserID, &res.CreationTime, &res.LastUpdated)
		if err != nil {
			return persist.Gallery{}, err
		}
		if res.ID != pID {
			return persist.Gallery{}, persist.ErrGalleryNotFoundByID{ID: pID}
		}
		return res, nil
	}

	for _, gallery := range galleries {
		collections := collections[gallery.ID]
		gallery.Collections = make([]persist.Collection, 0, len(collections))
		for _, coll := range collections {
			if coll.ID == "" {
				continue
			}
			gallery.Collections = append(gallery.Collections, coll)
		}
		return gallery, nil
	}
	return persist.Gallery{}, persist.ErrGalleryNotFoundByID{ID: pID}
}

// RefreshCache deletes the given key in the cache
func (g *GalleryRepository) RefreshCache(pCtx context.Context, pUserID persist.DBID) error {
	return g.galleriesCache.Delete(pCtx, pUserID.String())
}

func ensureCollsOwnedByUser(pCtx context.Context, g *GalleryRepository, pColls []persist.DBID, pUserID persist.DBID) error {
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

func ensureAllCollsAccountedFor(pCtx context.Context, g *GalleryRepository, pColls []persist.DBID, pUserID persist.DBID) ([]persist.DBID, error) {
	var ct int64
	err := g.countAllCollectionsStmt.QueryRowContext(pCtx, pUserID).Scan(&ct)
	if err != nil {
		return nil, err
	}
	if ct != int64(len(pColls)) {
		if int64(len(pColls)) < ct {
			return addUnaccountedForCollections(pCtx, g, pUserID, pColls)
		}
		return nil, errCollsNotOwnedByUser
	}
	return pColls, nil
}

func addUnaccountedForCollections(pCtx context.Context, g *GalleryRepository, pUserID persist.DBID, pColls []persist.DBID) ([]persist.DBID, error) {
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
