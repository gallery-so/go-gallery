//go:generate go run github.com/vektah/dataloaden UserLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/service/persist.User
//go:generate go run github.com/vektah/dataloaden UserLoaderByAddress github.com/mikeydub/go-gallery/service/persist.Address github.com/mikeydub/go-gallery/service/persist.User
//go:generate go run github.com/vektah/dataloaden UserLoaderByString string github.com/mikeydub/go-gallery/service/persist.User
//go:generate go run github.com/vektah/dataloaden GalleryLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/service/persist.Gallery
//go:generate go run github.com/vektah/dataloaden GalleriesLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/service/persist.Gallery
//go:generate go run github.com/vektah/dataloaden CollectionLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/service/persist.Collection
//go:generate go run github.com/vektah/dataloaden CollectionsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/service/persist.Collection
//go:generate go run github.com/vektah/dataloaden NftLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/service/persist.NFT
//go:generate go run github.com/vektah/dataloaden NftsLoaderByAddress github.com/mikeydub/go-gallery/service/persist.Address []github.com/mikeydub/go-gallery/service/persist.NFT

package dataloader

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"time"
)

const loadersKey = "graphql.dataloaders"
const defaultMaxBatch = 100
const defaultWaitTime = 1 * time.Millisecond

// Loaders will cache and batch lookups. They are short-lived and should never persist beyond
// a single request, nor should they be shared between requests (since the data returned is
// relative to the current request context, including the user and their auth status).
type Loaders struct {
	UserByUserId             UserLoaderByID
	UserByUsername           UserLoaderByString
	UserByAddress            UserLoaderByAddress
	GalleryByGalleryId       GalleryLoaderByID
	GalleriesByUserId        GalleriesLoaderByID
	CollectionByCollectionId CollectionLoaderByID
	CollectionsByUserId      CollectionsLoaderByID
	NftByNftId               NftLoaderByID
	NftsByAddress            NftsLoaderByAddress
}

func NewLoaders(ctx context.Context, r *persist.Repositories) *Loaders {
	loaders := &Loaders{}

	loaders.UserByUserId = UserLoaderByID{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadUserByUserId(ctx, loaders, r),
	}

	loaders.UserByUsername = UserLoaderByString{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadUserByUsername(ctx, loaders, r),
	}

	loaders.UserByAddress = UserLoaderByAddress{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadUserByAddress(ctx, loaders, r),
	}

	loaders.GalleryByGalleryId = GalleryLoaderByID{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadGalleryByGalleryId(ctx, loaders, r),
	}

	loaders.GalleriesByUserId = GalleriesLoaderByID{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadGalleriesByUserId(ctx, loaders, r),
	}

	loaders.CollectionByCollectionId = CollectionLoaderByID{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadCollectionByCollectionId(ctx, loaders, r),
	}

	loaders.CollectionsByUserId = CollectionsLoaderByID{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadCollectionsByUserId(ctx, loaders, r),
	}

	loaders.NftByNftId = NftLoaderByID{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadNftByNftId(ctx, loaders, r),
	}

	loaders.NftsByAddress = NftsLoaderByAddress{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadNftsByAddress(ctx, loaders, r),
	}

	return loaders
}

func loadUserByUserId(ctx context.Context, loaders *Loaders, r *persist.Repositories) func([]persist.DBID) ([]persist.User, []error) {
	return func(userIds []persist.DBID) ([]persist.User, []error) {
		// TODO: Add a new query to fetch all users at once so we can make use of batching.
		// Right now, the only benefit here is caching.
		users := make([]persist.User, len(userIds))
		errors := make([]error, len(userIds))

		for i, userId := range userIds {
			user, err := r.UserRepository.GetByID(ctx, userId)

			// Add results to other loaders' caches
			loaders.UserByUsername.Prime(user.Username.String(), user)
			for _, address := range user.Addresses {
				loaders.UserByAddress.Prime(address, user)
			}

			users[i], errors[i] = user, err
		}

		return users, errors
	}
}

func loadUserByUsername(ctx context.Context, loaders *Loaders, r *persist.Repositories) func([]string) ([]persist.User, []error) {
	return func(usernames []string) ([]persist.User, []error) {
		users := make([]persist.User, len(usernames))
		errors := make([]error, len(usernames))

		for i, username := range usernames {
			user, err := r.UserRepository.GetByUsername(ctx, username)

			// Add results to other loaders' caches
			loaders.UserByUserId.Prime(user.ID, user)
			for _, address := range user.Addresses {
				loaders.UserByAddress.Prime(address, user)
			}

			users[i], errors[i] = user, err
		}

		return users, errors
	}
}

func loadUserByAddress(ctx context.Context, loaders *Loaders, r *persist.Repositories) func([]persist.Address) ([]persist.User, []error) {
	return func(addresses []persist.Address) ([]persist.User, []error) {
		users := make([]persist.User, len(addresses))
		errors := make([]error, len(addresses))

		for i, address := range addresses {
			user, err := r.UserRepository.GetByAddress(ctx, persist.Address(address))
			// Add results to other loaders' caches
			loaders.UserByUserId.Prime(user.ID, user)
			loaders.UserByUsername.Prime(user.Username.String(), user)

			users[i], errors[i] = user, err
		}

		return users, errors
	}
}

func loadGalleryByGalleryId(ctx context.Context, loaders *Loaders, r *persist.Repositories) func([]persist.DBID) ([]persist.Gallery, []error) {
	return func(galleryIds []persist.DBID) ([]persist.Gallery, []error) {
		galleries := make([]persist.Gallery, len(galleryIds))
		errors := make([]error, len(galleryIds))

		for i, galleryId := range galleryIds {
			galleries[i], errors[i] = r.GalleryRepository.GetByID(ctx, galleryId)
		}

		return galleries, errors
	}
}

func loadGalleriesByUserId(ctx context.Context, loaders *Loaders, r *persist.Repositories) func([]persist.DBID) ([][]persist.Gallery, []error) {
	return func(userIds []persist.DBID) ([][]persist.Gallery, []error) {
		galleries := make([][]persist.Gallery, len(userIds))
		errors := make([]error, len(userIds))

		for i, userId := range userIds {
			galleries[i], errors[i] = r.GalleryRepository.GetByUserID(ctx, userId)

			// Add results to the GalleryByGalleryId loader's cache
			for _, gallery := range galleries[i] {
				loaders.GalleryByGalleryId.Prime(gallery.ID, gallery)
			}
		}

		return galleries, errors
	}
}

func loadCollectionByCollectionId(ctx context.Context, loaders *Loaders, r *persist.Repositories) func([]persist.DBID) ([]persist.Collection, []error) {
	return func(collectionIds []persist.DBID) ([]persist.Collection, []error) {
		collections := make([]persist.Collection, len(collectionIds))
		errors := make([]error, len(collectionIds))

		gc := util.GinContextFromContext(ctx)
		authed := auth.GetUserAuthedFromCtx(gc)

		for i, collectionId := range collectionIds {
			// Worth fixing in the future: "authed" actually checks whether the current user is logged in, so
			// any logged-in user can see another user's hidden collections. We'd probably want to move the
			// auth check into GetByID to see who owns the collection and determine whether it can be returned
			// to the requesting user.
			collections[i], errors[i] = r.CollectionRepository.GetByID(ctx, collectionId, authed)
		}

		return collections, errors
	}
}

func loadCollectionsByUserId(ctx context.Context, loaders *Loaders, r *persist.Repositories) func([]persist.DBID) ([][]persist.Collection, []error) {
	return func(userIds []persist.DBID) ([][]persist.Collection, []error) {
		collections := make([][]persist.Collection, len(userIds))
		errors := make([]error, len(userIds))

		gc := util.GinContextFromContext(ctx)
		authedUserId := auth.GetUserIDFromCtx(gc)

		for i, userId := range userIds {
			// GraphQL best practices would suggest moving this auth logic into the DB fetching layer
			// so it's applied consistently for all callers. It's probably worth doing at some point.
			showHidden := userId == authedUserId
			collections[i], errors[i] = r.CollectionRepository.GetByUserID(ctx, userId, showHidden)

			// Add results to the CollectionByCollectionId loader's cache
			for _, collection := range collections[i] {
				loaders.CollectionByCollectionId.Prime(collection.ID, collection)
			}
		}

		return collections, errors
	}
}

func loadNftsByAddress(ctx context.Context, loaders *Loaders, r *persist.Repositories) func([]persist.Address) ([][]persist.NFT, []error) {
	return func(addresses []persist.Address) ([][]persist.NFT, []error) {
		nfts := make([][]persist.NFT, len(addresses))
		errors := make([]error, len(addresses))

		for i, address := range addresses {
			nfts[i], errors[i] = r.NftRepository.GetByAddresses(ctx, []persist.Address{address})

			// Add results to the NftByNftId loader's cache
			for _, nft := range nfts[i] {
				loaders.NftByNftId.Prime(nft.ID, nft)
			}
		}

		return nfts, errors
	}
}

func loadNftByNftId(ctx context.Context, loaders *Loaders, r *persist.Repositories) func([]persist.DBID) ([]persist.NFT, []error) {
	return func(nftIds []persist.DBID) ([]persist.NFT, []error) {
		nfts := make([]persist.NFT, len(nftIds))
		errors := make([]error, len(nftIds))

		for i, nftId := range nftIds {
			nfts[i], errors[i] = r.NftRepository.GetByID(ctx, nftId)
		}

		return nfts, errors
	}
}

func AddTo(ctx *gin.Context, r *persist.Repositories) {
	ctx.Set(loadersKey, NewLoaders(ctx, r))
}

func For(ctx context.Context) *Loaders {
	gc := util.GinContextFromContext(ctx)
	return gc.Value(loadersKey).(*Loaders)
}
