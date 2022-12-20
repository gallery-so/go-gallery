package postgres

import (
	"context"
	"errors"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"

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
func (g *GalleryRepository) Create(pCtx context.Context, pGallery db.GalleryRepoCreateParams) (db.Gallery, error) {

	gal, err := g.queries.GalleryRepoCreate(pCtx, pGallery)
	if err != nil {
		return db.Gallery{}, err
	}

	return gal, nil
}

// Delete deletes a gallery and ensures that the collections of that gallery are passed on to another gallery
func (g *GalleryRepository) Delete(pCtx context.Context, pGallery db.GalleryRepoDeleteParams) error {

	err := g.queries.GalleryRepoDelete(pCtx, pGallery)
	if err != nil {
		return err
	}

	return nil
}

// Update updates the gallery with the given ID and ensures that gallery is owned by the given userID
func (g *GalleryRepository) Update(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate persist.GalleryTokenUpdateInput) error {
	err := ensureCollsOwnedByUserToken(pCtx, g, pUpdate.Collections, pUserID)
	if err != nil {
		return err
	}

	rowsAffected, err := g.queries.GalleryRepoUpdate(pCtx, db.GalleryRepoUpdateParams{
		CollectionIds: pUpdate.Collections,
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

// AddCollections adds the given collections to the gallery with the given ID
func (g *GalleryRepository) AddCollections(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pCollections []persist.DBID) error {

	err := ensureCollsOwnedByUserToken(pCtx, g, pCollections, pUserID)
	if err != nil {
		return err
	}

	rowsAffected, err := g.queries.GalleryRepoAddCollections(pCtx, db.GalleryRepoAddCollectionsParams{
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
