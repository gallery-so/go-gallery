//go:generate go run github.com/vektah/dataloaden UserLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc.User
//go:generate go run github.com/vektah/dataloaden UserLoaderByAddress github.com/mikeydub/go-gallery/service/persist.Address github.com/mikeydub/go-gallery/db/sqlc.User
//go:generate go run github.com/vektah/dataloaden UserLoaderByString string github.com/mikeydub/go-gallery/db/sqlc.User
//go:generate go run github.com/vektah/dataloaden GalleryLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc.Gallery
//go:generate go run github.com/vektah/dataloaden GalleriesLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/sqlc.Gallery
//go:generate go run github.com/vektah/dataloaden CollectionLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc.Collection
//go:generate go run github.com/vektah/dataloaden CollectionsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/sqlc.Collection
//go:generate go run github.com/vektah/dataloaden NftLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc.Nft
//go:generate go run github.com/vektah/dataloaden NftsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/sqlc.Nft
//go:generate go run github.com/vektah/dataloaden NftsLoaderByAddress github.com/mikeydub/go-gallery/service/persist.Address []github.com/mikeydub/go-gallery/db/sqlc.Nft

package dataloader

import (
	"context"
	"github.com/jackc/pgx/v4"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/service/persist"
	"time"
)

const defaultMaxBatch = 1 // Disable batching until loading functions support it
const defaultWaitTime = 1 * time.Millisecond

// Loaders will cache and batch lookups. They are short-lived and should never persist beyond
// a single request, nor should they be shared between requests (since the data returned is
// relative to the current request context, including the user and their auth status).
type Loaders struct {
	UserByUserId             UserLoaderByID
	UserByUsername           UserLoaderByString
	UserByAddress            UserLoaderByAddress
	GalleryByGalleryId       GalleryLoaderByID
	GalleryByCollectionId    GalleryLoaderByID
	GalleriesByUserId        GalleriesLoaderByID
	CollectionByCollectionId CollectionLoaderByID
	CollectionsByGalleryId   CollectionsLoaderByID
	NftByNftId               NftLoaderByID
	NftsByOwnerAddress       NftsLoaderByAddress
	NftsByCollectionId       NftsLoaderByID
}

func NewLoaders(ctx context.Context, q *sqlc.Queries) *Loaders {
	loaders := &Loaders{}

	loaders.UserByUserId = UserLoaderByID{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadUserByUserId(ctx, loaders, q),
	}

	loaders.UserByUsername = UserLoaderByString{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadUserByUsername(ctx, loaders, q),
	}

	loaders.UserByAddress = UserLoaderByAddress{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadUserByAddress(ctx, loaders, q),
	}

	loaders.GalleryByGalleryId = GalleryLoaderByID{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadGalleryByGalleryId(ctx, loaders, q),
	}

	loaders.GalleryByCollectionId = GalleryLoaderByID{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadGalleryByCollectionId(ctx, loaders, q),
	}

	loaders.GalleriesByUserId = GalleriesLoaderByID{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadGalleriesByUserId(ctx, loaders, q),
	}

	loaders.CollectionByCollectionId = CollectionLoaderByID{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadCollectionByCollectionId(ctx, loaders, q),
	}

	loaders.CollectionsByGalleryId = CollectionsLoaderByID{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadCollectionsByGalleryId(ctx, loaders, q),
	}

	loaders.NftByNftId = NftLoaderByID{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadNftByNftId(ctx, loaders, q),
	}

	loaders.NftsByOwnerAddress = NftsLoaderByAddress{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadNftsByOwnerAddress(ctx, loaders, q),
	}

	loaders.NftsByCollectionId = NftsLoaderByID{
		maxBatch: defaultMaxBatch,
		wait:     defaultWaitTime,
		fetch:    loadNftsByCollectionId(ctx, loaders, q),
	}

	return loaders
}

func loadUserByUserId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([]sqlc.User, []error) {
	return func(userIds []persist.DBID) ([]sqlc.User, []error) {
		users := make([]sqlc.User, len(userIds))
		errors := make([]error, len(userIds))

		for i, userId := range userIds {
			user, err := q.GetUserById(ctx, userId)

			if err == pgx.ErrNoRows {
				err = persist.ErrUserNotFound{UserID: userId}
			}

			// Add results to other loaders' caches
			if err == nil {
				loaders.UserByUsername.Prime(user.Username.String, user)
				for _, address := range user.Addresses {
					loaders.UserByAddress.Prime(address, user)
				}
			}

			users[i], errors[i] = user, err
		}

		return users, errors
	}
}

func loadUserByUsername(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]string) ([]sqlc.User, []error) {
	return func(usernames []string) ([]sqlc.User, []error) {
		users := make([]sqlc.User, len(usernames))
		errors := make([]error, len(usernames))

		for i, username := range usernames {
			user, err := q.GetUserByUsername(ctx, username)

			if err == pgx.ErrNoRows {
				err = persist.ErrUserNotFound{Username: username}
			}

			// Add results to other loaders' caches
			if err == nil {
				loaders.UserByUserId.Prime(user.ID, user)
				for _, address := range user.Addresses {
					loaders.UserByAddress.Prime(address, user)
				}
			}

			users[i], errors[i] = user, err
		}

		return users, errors
	}
}

func loadUserByAddress(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.Address) ([]sqlc.User, []error) {
	return func(addresses []persist.Address) ([]sqlc.User, []error) {
		users := make([]sqlc.User, len(addresses))
		errors := make([]error, len(addresses))

		for i, address := range addresses {
			user, err := q.GetUserByAddress(ctx, address.String())
			if err == pgx.ErrNoRows {
				err = persist.ErrUserNotFound{Address: address}
			}

			// Add results to other loaders' caches
			if err == nil {
				loaders.UserByUserId.Prime(user.ID, user)
				loaders.UserByUsername.Prime(user.Username.String, user)
			}

			users[i], errors[i] = user, err
		}

		return users, errors
	}
}

func loadGalleryByGalleryId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([]sqlc.Gallery, []error) {
	return func(galleryIds []persist.DBID) ([]sqlc.Gallery, []error) {
		galleries := make([]sqlc.Gallery, len(galleryIds))
		errors := make([]error, len(galleryIds))

		for i, galleryId := range galleryIds {
			galleries[i], errors[i] = q.GetGalleryById(ctx, galleryId)
			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrGalleryNotFoundByID{ID: galleryId}
			}

			// Add results to other loaders' caches
			if errors[i] == nil {
				for _, collection := range galleries[i].Collections {
					loaders.GalleryByCollectionId.Prime(collection, galleries[i])
				}
			}
		}

		return galleries, errors
	}
}

func loadGalleryByCollectionId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([]sqlc.Gallery, []error) {
	return func(collectionIds []persist.DBID) ([]sqlc.Gallery, []error) {
		galleries := make([]sqlc.Gallery, len(collectionIds))
		errors := make([]error, len(collectionIds))

		for i, collectionId := range collectionIds {
			galleries[i], errors[i] = q.GetGalleryByCollectionId(ctx, collectionId)
			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrGalleryNotFoundByCollectionID{ID: collectionId}
			}

			// Add results to other loaders' caches
			if errors[i] == nil {
				loaders.GalleryByGalleryId.Prime(galleries[i].ID, galleries[i])
			}
		}

		return galleries, errors
	}
}

func loadGalleriesByUserId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([][]sqlc.Gallery, []error) {
	return func(userIds []persist.DBID) ([][]sqlc.Gallery, []error) {
		galleries := make([][]sqlc.Gallery, len(userIds))
		errors := make([]error, len(userIds))

		for i, userId := range userIds {
			galleries[i], errors[i] = q.GetGalleriesByUserId(ctx, userId)

			// Add results to other loaders' caches
			if errors[i] == nil {
				for _, gallery := range galleries[i] {
					loaders.GalleryByGalleryId.Prime(gallery.ID, gallery)
					for _, collection := range gallery.Collections {
						loaders.GalleryByCollectionId.Prime(collection, gallery)
					}
				}
			}
		}

		return galleries, errors
	}
}

func loadCollectionByCollectionId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([]sqlc.Collection, []error) {
	return func(collectionIds []persist.DBID) ([]sqlc.Collection, []error) {
		collections := make([]sqlc.Collection, len(collectionIds))
		errors := make([]error, len(collectionIds))

		for i, collectionId := range collectionIds {
			collections[i], errors[i] = q.GetCollectionById(ctx, collectionId)
			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrCollectionNotFoundByID{ID: collectionId}
			}
		}

		return collections, errors
	}
}

func loadCollectionsByGalleryId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([][]sqlc.Collection, []error) {
	return func(galleryIds []persist.DBID) ([][]sqlc.Collection, []error) {
		collections := make([][]sqlc.Collection, len(galleryIds))
		errors := make([]error, len(galleryIds))

		for i, galleryId := range galleryIds {
			collections[i], errors[i] = q.GetCollectionsByGalleryId(ctx, galleryId)

			// Add results to the CollectionByCollectionId loader's cache
			if errors[i] == nil {
				for _, collection := range collections[i] {
					loaders.CollectionByCollectionId.Prime(collection.ID, collection)
				}
			}
		}

		return collections, errors
	}
}

func loadNftByNftId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([]sqlc.Nft, []error) {
	return func(nftIds []persist.DBID) ([]sqlc.Nft, []error) {
		nfts := make([]sqlc.Nft, len(nftIds))
		errors := make([]error, len(nftIds))

		for i, nftId := range nftIds {
			nfts[i], errors[i] = q.GetNftById(ctx, nftId)
			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrNFTNotFoundByID{ID: nftId}
			}
		}

		return nfts, errors
	}
}

func loadNftsByOwnerAddress(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.Address) ([][]sqlc.Nft, []error) {
	return func(addresses []persist.Address) ([][]sqlc.Nft, []error) {
		nfts := make([][]sqlc.Nft, len(addresses))
		errors := make([]error, len(addresses))

		for i, address := range addresses {
			nfts[i], errors[i] = q.GetNftsByOwnerAddress(ctx, address)

			// Add results to the NftByNftId loader's cache
			if errors[i] == nil {
				for _, nft := range nfts[i] {
					loaders.NftByNftId.Prime(nft.ID, nft)
				}
			}
		}

		return nfts, errors
	}
}

func loadNftsByCollectionId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([][]sqlc.Nft, []error) {
	return func(collectionIds []persist.DBID) ([][]sqlc.Nft, []error) {
		nfts := make([][]sqlc.Nft, len(collectionIds))
		errors := make([]error, len(collectionIds))

		for i, collectionId := range collectionIds {
			nfts[i], errors[i] = q.GetNftsByCollectionId(ctx, collectionId)

			// Add results to the NftByNftId loader's cache
			if errors[i] == nil {
				for _, nft := range nfts[i] {
					loaders.NftByNftId.Prime(nft.ID, nft)
				}
			}
		}

		return nfts, errors
	}
}
