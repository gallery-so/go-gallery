package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"time"

	"github.com/mikeydub/go-gallery/service/logger"

	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/persist"
)

const galleryCacheTime = time.Hour * 24 * 3

var errCollsNotOwnedByUser = errors.New("collections not owned by user")

// GalleryRepository is the repository for interacting with galleries in a postgres database
type GalleryRepository struct {
	db              *sql.DB
	queries         *db.Queries
	getByUserIDStmt *sql.Stmt

	galleriesCache memstore.Cache
}

// NewGalleryRepository creates a new GalleryTokenRepository
// TODO another join to addresses
func NewGalleryRepository(db *sql.DB, queries *db.Queries, gCache memstore.Cache) *GalleryRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	getByUserIDStmt, err := db.PrepareContext(ctx, `SELECT g.ID,g.VERSION,g.OWNER_USER_ID,g.CREATED_AT,g.LAST_UPDATED,
	c.ID,c.OWNER_USER_ID,c.NAME,c.VERSION,c.DELETED,c.COLLECTORS_NOTE,c.LAYOUT,c.HIDDEN,c.CREATED_AT,c.LAST_UPDATED,
	n.ID,n.OWNER_USER_ID,n.CHAIN,n.NAME,n.DESCRIPTION,n.TOKEN_TYPE,n.TOKEN_URI,n.TOKEN_ID,n.MEDIA,n.TOKEN_METADATA,n.CONTRACT,n.CREATED_AT 
	FROM galleries g, unnest(g.COLLECTIONS) WITH ORDINALITY AS u(coll, coll_ord)
	LEFT JOIN collections c ON c.ID = coll AND c.DELETED = false
	LEFT JOIN LATERAL (SELECT n.*,nft,nft_ord FROM tokens n, unnest(c.NFTS) WITH ORDINALITY AS x(nft, nft_ord)) n ON n.ID = n.nft
	WHERE g.OWNER_USER_ID = $1 AND g.DELETED = false ORDER BY coll_ord,n.nft_ord;`)
	checkNoErr(err)

	return &GalleryRepository{db: db, queries: queries, getByUserIDStmt: getByUserIDStmt, galleriesCache: gCache}
}

// Create creates a new gallery
func (g *GalleryRepository) Create(pCtx context.Context, pGallery persist.GalleryDB) (persist.DBID, error) {

	err := ensureCollsOwnedByUserToken(pCtx, g, pGallery.Collections, pGallery.OwnerUserID)
	if err != nil {
		return "", err
	}

	colls, err := ensureAllCollsAccountedForToken(pCtx, g, pGallery.Collections, pGallery.OwnerUserID)
	if err != nil {
		return "", err
	}
	pGallery.Collections = colls

	id, err := g.queries.GalleryRepoCreate(pCtx, db.GalleryRepoCreateParams{
		ID:          persist.GenerateID(),
		Version:     sql.NullInt32{Int32: pGallery.Version.Int32(), Valid: true},
		Collections: pGallery.Collections,
		OwnerUserID: pGallery.OwnerUserID,
	})

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
func (g *GalleryRepository) Update(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate persist.GalleryTokenUpdateInput) error {
	err := ensureCollsOwnedByUserToken(pCtx, g, pUpdate.Collections, pUserID)
	if err != nil {
		return err
	}
	colls, err := ensureAllCollsAccountedForToken(pCtx, g, pUpdate.Collections, pUserID)
	if err != nil {
		return err
	}
	pUpdate.Collections = colls

	rowsAffected, err := g.queries.GalleryRepoUpdate(pCtx, db.GalleryRepoUpdateParams{
		LastUpdated: time.Time(pUpdate.LastUpdated),
		Collections: pUpdate.Collections,
		GalleryID:   pID,
		OwnerUserID: pUserID,
	})

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
//func (g *GalleryRepository) UpdateUnsafe(pCtx context.Context, pID persist.DBID, pUpdate persist.GalleryTokenUpdateInput) error {
//	res, err := g.updateUnsafeStmt.ExecContext(pCtx, pUpdate.LastUpdated, pq.Array(pUpdate.Collections), pID)
//	if err != nil {
//		return err
//	}
//	rowsAffected, err := res.RowsAffected()
//	if err != nil {
//		return err
//	}
//	if rowsAffected == 0 {
//		return persist.ErrGalleryNotFoundByID{ID: pID}
//	}
//	err = g.cacheByID(pCtx, pID)
//	if err != nil {
//		return err
//	}
//	return nil
//}

// AddCollections adds the given collections to the gallery with the given ID
func (g *GalleryRepository) AddCollections(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pCollections []persist.DBID) error {

	err := ensureCollsOwnedByUserToken(pCtx, g, pCollections, pUserID)
	if err != nil {
		return err
	}

	ct, err := g.queries.GalleryRepoCountColls(pCtx, pID)
	if err != nil {
		return err
	}

	allCollsCt, err := g.queries.GalleryRepoCountAllCollections(pCtx, pUserID)
	if err != nil {
		return err
	}

	if ct+int64(len(pCollections)) != allCollsCt {
		galleryCollIDs, err := g.queries.GalleryRepoGetGalleryCollections(pCtx, pID)
		if err != nil {
			return err
		}
		galleryCollIDs = append(pCollections, galleryCollIDs...)

		allColls, err := addUnaccountedForCollectionsToken(pCtx, g, pUserID, galleryCollIDs)
		if err != nil {
			return err
		}

		rowsAffected, err := g.queries.GalleryRepoUpdate(pCtx, db.GalleryRepoUpdateParams{
			LastUpdated: time.Now(),
			Collections: allColls,
			GalleryID:   pID,
			OwnerUserID: pUserID,
		})

		if err != nil {
			return err
		}

		if rowsAffected == 0 {
			return persist.ErrGalleryNotFoundByID{ID: pID}
		}

		return nil
	}

	rowsAffected, err := g.queries.GalleryRepoAddCollections(pCtx, db.GalleryRepoAddCollectionsParams{
		OwnerUserID:   pUserID,
		CollectionIds: dbidsToStrings(pCollections),
		GalleryID:     pID,
	})

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
// TODO: Examine uses of this, since we don't return galleries in this hydrated format anymore.
// 		 Ideally, we can replace this function with non-hydrates functions.
func (g *GalleryRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID) ([]persist.Gallery, error) {
	if g.galleriesCache != nil {
		initial, _ := g.galleriesCache.Get(pCtx, pUserID.String())
		if len(initial) > 0 {
			var galleries []persist.Gallery
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

	galleries := make(map[persist.DBID]persist.Gallery)
	collections := make(map[persist.DBID][]persist.Collection)
	var gallery persist.Gallery
	var lastCollID persist.DBID
	for rows.Next() {
		var collection persist.Collection
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
			colls = make([]persist.Collection, 0, 10)

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

	result := make([]persist.Gallery, 0, len(galleries))

	if len(galleries) == 0 {
		galleriesRaw, err := g.queries.GalleryRepoGetByUserIDRaw(pCtx, pUserID)
		if err != nil {
			return nil, err
		}

		for _, galleryRaw := range galleriesRaw {
			result = append(result, persist.Gallery{
				Version:      persist.NullInt32(galleryRaw.Version.Int32),
				ID:           galleryRaw.ID,
				CreationTime: persist.CreationTime(galleryRaw.CreatedAt),
				Deleted:      persist.NullBool(galleryRaw.Deleted),
				LastUpdated:  persist.LastUpdatedTime(galleryRaw.LastUpdated),
				OwnerUserID:  galleryRaw.OwnerUserID,
				Collections:  []persist.Collection{},
			})
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
//func (g *GalleryRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.Gallery, error) {
//	rows, err := g.getByIDStmt.QueryContext(pCtx, pID)
//	if err != nil {
//		return persist.Gallery{}, err
//	}
//	defer rows.Close()
//
//	galleries := make(map[persist.DBID]persist.Gallery)
//	collections := make(map[persist.DBID][]persist.Collection)
//	var gallery persist.Gallery
//	var lastCollID persist.DBID
//	for rows.Next() {
//		var collection persist.Collection
//		var nft persist.TokenInCollection
//
//		err := rows.Scan(&gallery.ID, &gallery.Version, &gallery.OwnerUserID, &gallery.CreationTime, &gallery.LastUpdated,
//			&collection.ID, &collection.OwnerUserID, &collection.Name, &collection.Version, &collection.Deleted, &collection.CollectorsNote,
//			&collection.Layout, &collection.Hidden, &collection.CreationTime, &collection.LastUpdated, &nft.ID, &nft.OwnerAddress,
//			&nft.Chain, &nft.Name, &nft.Description, &nft.TokenType, &nft.TokenURI, &nft.TokenID, &nft.Media, &nft.TokenMetadata, &nft.ContractAddress, &nft.CreationTime)
//		if err != nil {
//			return persist.Gallery{}, err
//		}
//		if _, ok := galleries[gallery.ID]; !ok {
//			galleries[gallery.ID] = gallery
//		}
//
//		if collection.ID == "" {
//			continue
//		}
//		colls, ok := collections[gallery.ID]
//		if !ok {
//			colls = make([]persist.Collection, 0, 10)
//
//		}
//		if lastCollID != collection.ID {
//			if nft.ID != "" {
//				collection.NFTs = []persist.TokenInCollection{nft}
//			} else {
//				collection.NFTs = []persist.TokenInCollection{}
//			}
//			colls = append(colls, collection)
//		} else {
//			lastColl := colls[len(colls)-1]
//			lastColl.NFTs = append(lastColl.NFTs, nft)
//			colls[len(colls)-1] = lastColl
//
//		}
//
//		collections[gallery.ID] = colls
//		lastCollID = collection.ID
//	}
//	if err := rows.Err(); err != nil {
//		return persist.Gallery{}, err
//	}
//
//	if len(galleries) > 1 {
//		return persist.Gallery{}, errors.New("too many galleries")
//	}
//
//	if len(galleries) == 0 {
//		res := persist.Gallery{Collections: []persist.Collection{}}
//		err := g.getByUserIDRawStmt.QueryRowContext(pCtx, pID).Scan(&res.ID, &res.Version, &res.OwnerUserID, &res.CreationTime, &res.LastUpdated)
//		if err != nil {
//			return persist.Gallery{}, err
//		}
//		if res.ID != pID {
//			return persist.Gallery{}, persist.ErrGalleryNotFoundByID{ID: pID}
//		}
//		return res, nil
//	}
//
//	for _, gallery := range galleries {
//		collections := collections[gallery.ID]
//		gallery.Collections = make([]persist.Collection, 0, len(collections))
//		for _, coll := range collections {
//			if coll.ID == "" {
//				continue
//			}
//			gallery.Collections = append(gallery.Collections, coll)
//		}
//		return gallery, nil
//	}
//	return persist.Gallery{}, persist.ErrGalleryNotFoundByID{ID: pID}
//}

// RefreshCache deletes the given key in the cache
func (g *GalleryRepository) RefreshCache(pCtx context.Context, pUserID persist.DBID) error {
	return g.galleriesCache.Delete(pCtx, pUserID.String())
}

func ensureCollsOwnedByUserToken(pCtx context.Context, g *GalleryRepository, pColls []persist.DBID, pUserID persist.DBID) error {
	numOwned, err := g.queries.GalleryRepoCheckOwnCollections(pCtx, db.GalleryRepoCheckOwnCollectionsParams{
		CollectionIds: pColls,
		OwnerUserID:   pUserID,
	})

	if err != nil {
		return err
	}

	if numOwned != int64(len(pColls)) {
		return errCollsNotOwnedByUser
	}

	return nil
}

func ensureAllCollsAccountedForToken(pCtx context.Context, g *GalleryRepository, pColls []persist.DBID, pUserID persist.DBID) ([]persist.DBID, error) {
	ct, err := g.queries.GalleryRepoCountAllCollections(pCtx, pUserID)
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

func addUnaccountedForCollectionsToken(pCtx context.Context, g *GalleryRepository, pUserID persist.DBID, pColls []persist.DBID) ([]persist.DBID, error) {
	colls, err := g.queries.GalleryRepoGetCollections(pCtx, pUserID)
	if err != nil {
		return nil, err
	}
	return appendDifference(pColls, colls), nil
}

//func (g *GalleryRepository) cacheByID(pCtx context.Context, pID persist.DBID) error {
//	gal, err := g.GetByID(pCtx, pID)
//	if err != nil {
//		return err
//	}
//	gals, err := g.GetByUserID(pCtx, gal.OwnerUserID)
//	if err != nil {
//		return err
//	}
//	marshalled, err := json.Marshal(gals)
//	if err != nil {
//		return err
//	}
//	if err = g.galleriesCache.Set(pCtx, gal.OwnerUserID.String(), marshalled, -1); err != nil {
//		return err
//	}
//	return nil
//}

func (g *GalleryRepository) cacheByUserID(pCtx context.Context, pUserID persist.DBID) error {
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
