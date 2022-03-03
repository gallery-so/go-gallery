package publicapi

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/pubsub"
	"github.com/mikeydub/go-gallery/util"
)

const maxCollectionsPerGallery = 1000

// TODO: Convert this to a validation error
var errTooManyCollectionsInGallery = errors.New(fmt.Sprintf("maximum of %d collections in a gallery", maxCollectionsPerGallery))

type GalleryAPI struct {
	repos     *persist.Repositories
	loaders   *dataloader.Loaders
	ethClient *ethclient.Client
	pubsub    pubsub.PubSub
}

func (api GalleryAPI) UpdateGalleryCollections(ctx context.Context, galleryID persist.DBID, collections []persist.DBID) error {
	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	if len(collections) > maxCollectionsPerGallery {
		// TODO: Validation error
		return errTooManyCollectionsInGallery
	}

	// TODO: Is this something we should be handling gracefully, or can we throw a validation error when we see a duplicate?
	collections = persist.RemoveDuplicateDBIDs(collections)

	update := persist.GalleryUpdateInput{Collections: collections}

	err = api.repos.GalleryRepository.Update(ctx, galleryID, userID, update)
	if err != nil {
		return err
	}

	backupGalleriesForUser(ctx, userID, api.repos)

	return nil
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
