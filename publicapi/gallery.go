package publicapi

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/fingerprints"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

const maxCollectionsPerGallery = 1000

type GalleryAPI struct {
	repos     *persist.Repositories
	queries   *db.Queries
	loaders   *dataloader.Loaders
	validator *validator.Validate
	ethClient *ethclient.Client
}

func (api GalleryAPI) GetGalleryById(ctx context.Context, galleryID persist.DBID) (*db.Gallery, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"galleryID": {galleryID, "required"},
	}); err != nil {
		return nil, err
	}

	gallery, err := api.loaders.GalleryByGalleryID.Load(galleryID)
	if err != nil {
		return nil, err
	}

	return &gallery, nil
}

func (api GalleryAPI) GetGalleryByCollectionId(ctx context.Context, collectionID persist.DBID) (*db.Gallery, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"collectionID": {collectionID, "required"},
	}); err != nil {
		return nil, err
	}

	gallery, err := api.loaders.GalleryByCollectionID.Load(collectionID)
	if err != nil {
		return nil, err
	}

	return &gallery, nil
}

func (api GalleryAPI) GetGalleriesByUserId(ctx context.Context, userID persist.DBID) ([]db.Gallery, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, err
	}

	galleries, err := api.loaders.GalleriesByUserID.Load(userID)
	if err != nil {
		return nil, err
	}

	return galleries, nil
}

func (api GalleryAPI) UpdateGalleryCollections(ctx context.Context, galleryID persist.DBID, collections []persist.DBID) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"galleryID":   {galleryID, "required"},
		"collections": {collections, fmt.Sprintf("required,unique,max=%d", maxCollectionsPerGallery)},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	update := persist.GalleryTokenUpdateInput{Collections: collections}

	err = api.repos.GalleryRepository.Update(ctx, galleryID, userID, update)
	if err != nil {
		return err
	}

	backupGalleriesForUser(ctx, userID, api.repos)

	return nil
}

func (api GalleryAPI) ViewGallery(ctx context.Context, galleryID persist.DBID) (db.Gallery, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"galleryID": {galleryID, "required"},
	}); err != nil {
		return db.Gallery{}, err
	}

	gallery, err := api.loaders.GalleryByGalleryID.Load(galleryID)
	if err != nil {
		return db.Gallery{}, err
	}

	gc := util.GinContextFromContext(ctx)

	if auth.GetUserAuthedFromCtx(gc) {
		userID, err := getAuthenticatedUser(ctx)
		if err != nil {
			return db.Gallery{}, err
		}

		if gallery.OwnerUserID != userID {
			// only view gallery if the user hasn't already viewed it in this most recent notification period

			dispatchEvent(ctx, db.Event{
				ActorID:        userID,
				ResourceTypeID: persist.ResourceTypeGallery,
				SubjectID:      galleryID,
				Action:         persist.ActionViewedGallery,
				GalleryID:      galleryID,
			})
		}
	} else {
		fp, err := fingerprints.GetFingerprintFromCtx(gc)
		if err != nil {
			return db.Gallery{}, err
		}

		dispatchEvent(ctx, db.Event{
			ResourceTypeID: persist.ResourceTypeGallery,
			SubjectID:      galleryID,
			Action:         persist.ActionViewedGallery,
			GalleryID:      galleryID,
			Fingerprint:    fp,
		})
	}

	return gallery, nil
}

func backupGalleriesForUser(ctx context.Context, userID persist.DBID, repos *persist.Repositories) {
	ctxCopy := util.GinContextFromContext(ctx).Copy()

	// TODO: Make sure backups still work here with our gin context retrieval
	go func(ctx context.Context) {
		galleries, err := repos.GalleryRepository.GetByUserID(ctx, userID)
		if err != nil {
			return
		}

		for _, gallery := range galleries {
			repos.BackupRepository.Insert(ctx, gallery)
		}
	}(ctxCopy)
}
