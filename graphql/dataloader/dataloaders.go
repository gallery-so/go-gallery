//go:generate go run github.com/vektah/dataloaden UserLoader string github.com/mikeydub/go-gallery/service/persist.User
//go:generate go run github.com/vektah/dataloaden GalleryLoader string github.com/mikeydub/go-gallery/service/persist.Gallery
//go:generate go run github.com/vektah/dataloaden GalleriesLoader string []github.com/mikeydub/go-gallery/service/persist.Gallery
//go:generate go run github.com/vektah/dataloaden CollectionLoader string github.com/mikeydub/go-gallery/service/persist.Collection
//go:generate go run github.com/vektah/dataloaden CollectionsLoader string []github.com/mikeydub/go-gallery/service/persist.Collection

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
	UserByUserId             UserLoader
	UserByUsername           UserLoader
	UserByAddress            UserLoader
	GalleryByGalleryId       GalleryLoader
	GalleriesByUserId        GalleriesLoader
	CollectionByCollectionId CollectionLoader
	CollectionsByUserId      CollectionsLoader
}

func NewLoaders(ctx context.Context, r *persist.Repositories) *Loaders {
	loaders := &Loaders{}

	loaders.UserByUserId = UserLoader{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadUserByUserId(ctx, loaders, r),
	}

	loaders.UserByUsername = UserLoader{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadUserByUsername(ctx, loaders, r),
	}

	loaders.UserByAddress = UserLoader{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadUserByAddress(ctx, loaders, r),
	}

	loaders.GalleryByGalleryId = GalleryLoader{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadGalleryByGalleryId(ctx, loaders, r),
	}

	loaders.GalleriesByUserId = GalleriesLoader{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadGalleriesByUserId(ctx, loaders, r),
	}

	loaders.CollectionByCollectionId = CollectionLoader{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadCollectionByCollectionId(ctx, loaders, r),
	}

	loaders.CollectionsByUserId = CollectionsLoader{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadCollectionsByUserId(ctx, loaders, r),
	}

	return loaders
}

func loadUserByUserId(ctx context.Context, loaders *Loaders, r *persist.Repositories) func([]string) ([]persist.User, []error) {
	return func(userIds []string) ([]persist.User, []error) {
		// TODO: Add a new query to fetch all users at once so we can make use of batching.
		// Right now, the only benefit here is caching.
		users := make([]persist.User, len(userIds))
		errors := make([]error, len(userIds))

		for i, userId := range userIds {
			user, err := r.UserRepository.GetByID(ctx, persist.DBID(userId))

			// Add results to other loaders' caches
			loaders.UserByUsername.Prime(user.Username.String(), user)
			for _, address := range user.Addresses {
				loaders.UserByAddress.Prime(address.String(), user)
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
			loaders.UserByUserId.Prime(user.ID.String(), user)
			for _, address := range user.Addresses {
				loaders.UserByAddress.Prime(address.String(), user)
			}

			users[i], errors[i] = user, err
		}

		return users, errors
	}
}

func loadUserByAddress(ctx context.Context, loaders *Loaders, r *persist.Repositories) func([]string) ([]persist.User, []error) {
	return func(addresses []string) ([]persist.User, []error) {
		users := make([]persist.User, len(addresses))
		errors := make([]error, len(addresses))

		for i, address := range addresses {
			user, err := r.UserRepository.GetByAddress(ctx, persist.Address(address))
			// Add results to other loaders' caches
			loaders.UserByUserId.Prime(user.ID.String(), user)
			loaders.UserByUsername.Prime(user.Username.String(), user)

			users[i], errors[i] = user, err
		}

		return users, errors
	}
}

func loadGalleryByGalleryId(ctx context.Context, loaders *Loaders, r *persist.Repositories) func([]string) ([]persist.Gallery, []error) {
	return func(galleryIds []string) ([]persist.Gallery, []error) {
		galleries := make([]persist.Gallery, len(galleryIds))
		errors := make([]error, len(galleryIds))

		for i, galleryId := range galleryIds {
			galleries[i], errors[i] = r.GalleryRepository.GetByID(ctx, persist.DBID(galleryId))
		}

		return galleries, errors
	}
}

func loadGalleriesByUserId(ctx context.Context, loaders *Loaders, r *persist.Repositories) func([]string) ([][]persist.Gallery, []error) {
	return func(userIds []string) ([][]persist.Gallery, []error) {
		galleries := make([][]persist.Gallery, len(userIds))
		errors := make([]error, len(userIds))

		for i, userId := range userIds {
			galleries[i], errors[i] = r.GalleryRepository.GetByUserID(ctx, persist.DBID(userId))

			// Add results to the GalleryByGalleryId loader's cache
			for _, gallery := range galleries[i] {
				loaders.GalleryByGalleryId.Prime(gallery.ID.String(), gallery)
			}
		}

		return galleries, errors
	}
}

func loadCollectionByCollectionId(ctx context.Context, loaders *Loaders, r *persist.Repositories) func([]string) ([]persist.Collection, []error) {
	return func(collectionIds []string) ([]persist.Collection, []error) {
		collections := make([]persist.Collection, len(collectionIds))
		errors := make([]error, len(collectionIds))

		gc := util.GinContextFromContext(ctx)
		authed := auth.GetUserAuthedFromCtx(gc)

		for i, collectionId := range collectionIds {
			// Worth fixing in the future: "authed" actually checks whether the current user is logged in, so
			// any logged-in user can see another user's hidden collections. We'd probably want to move the
			// auth check into GetByID to see who owns the collection and determine whether it can be returned
			// to the requesting user.
			collections[i], errors[i] = r.CollectionRepository.GetByID(ctx, persist.DBID(collectionId), authed)
		}

		return collections, errors
	}
}

func loadCollectionsByUserId(ctx context.Context, loaders *Loaders, r *persist.Repositories) func([]string) ([][]persist.Collection, []error) {
	return func(userIds []string) ([][]persist.Collection, []error) {
		collections := make([][]persist.Collection, len(userIds))
		errors := make([]error, len(userIds))

		gc := util.GinContextFromContext(ctx)
		authedUserId := auth.GetUserIDFromCtx(gc).String()

		for i, userId := range userIds {
			// GraphQL best practices would suggest moving this auth logic into the DB fetching layer
			// so it's applied consistently for all callers. It's probably worth doing at some point.
			showHidden := userId == authedUserId
			collections[i], errors[i] = r.CollectionRepository.GetByUserID(ctx, persist.DBID(userId), showHidden)

			// Add results to the CollectionByCollectionId loader's cache
			for _, collection := range collections[i] {
				loaders.CollectionByCollectionId.Prime(collection.ID.String(), collection)
			}
		}

		return collections, errors
	}
}

func AddTo(ctx *gin.Context, r *persist.Repositories) {
	ctx.Set(loadersKey, NewLoaders(ctx, r))
}

func For(ctx context.Context) *Loaders {
	gc := util.GinContextFromContext(ctx)
	return gc.Value(loadersKey).(*Loaders)
}
