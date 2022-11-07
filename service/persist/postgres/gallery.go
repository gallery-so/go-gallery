package postgres

import (
	"context"
	"database/sql"
	"errors"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

var errCollsNotOwnedByUser = errors.New("collections not owned by user")

// GalleryRepository is the repository for interacting with galleries in a postgres database
type GalleryRepository struct {
	queries *db.Queries
}

// NewGalleryRepository creates a new GalleryTokenRepository
// TODO another join to addresses
func NewGalleryRepository(queries *db.Queries) *GalleryRepository {
	return &GalleryRepository{queries: queries}
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

	return nil
}

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

	return nil
}

func (g *GalleryRepository) GetPreviewsURLsByUserID(pCtx context.Context, pUserID persist.DBID, limit int) ([]string, error) {
	return g.queries.GalleryRepoGetPreviewsForUserID(pCtx, db.GalleryRepoGetPreviewsForUserIDParams{
		OwnerUserID: pUserID,
		Limit:       int32(limit),
	})
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
