//go:generate go run github.com/vektah/dataloaden UserLoader string github.com/mikeydub/go-gallery/service/persist.User
//go:generate go run github.com/vektah/dataloaden GalleryLoader string github.com/mikeydub/go-gallery/service/persist.Gallery
//go:generate go run github.com/vektah/dataloaden GalleriesLoader string []github.com/mikeydub/go-gallery/service/persist.Gallery

package dataloader

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"time"
)

const loadersKey = "graphql.dataloaders"
const defaultMaxBatch = 100
const defaultWaitTime = 1 * time.Millisecond

type Loaders struct {
	UserByUserId       UserLoader
	UserByUsername     UserLoader
	UserByAddress      UserLoader
	GalleryByGalleryId GalleryLoader
	GalleriesByUserId  GalleriesLoader
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

func AddTo(ctx *gin.Context, r *persist.Repositories) {
	ctx.Set(loadersKey, NewLoaders(ctx, r))
}

func For(ctx context.Context) *Loaders {
	gc := util.GinContextFromContext(ctx)
	return gc.Value(loadersKey).(*Loaders)
}

//func Middleware(conn *sql.DB, next http.Handler) http.Handler {
//	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		ctx := context.WithValue(r.Context(), loadersKey, &Loaders{
//			UserById: UserLoader{
//				maxBatch: 100,
//				wait:     1 * time.Millisecond,
//				fetch: func(ids []int) ([]*model.User, []error) {
//					placeholders := make([]string, len(ids))
//					args := make([]interface{}, len(ids))
//					for i := 0; i < len(ids); i++ {
//						placeholders[i] = "?"
//						args[i] = i
//					}
//
//					res := db.LogAndQuery(conn,
//						"SELECT id, name from dataloader_example.user WHERE id IN ("+strings.Join(placeholders, ",")+")",
//						args...,
//					)
//					defer res.Close()
//
//					userById := map[int]*model.User{}
//					for res.Next() {
//						user := model.User{}
//						err := res.Scan(&user.ID, &user.Name)
//						if err != nil {
//							panic(err)
//						}
//						userById[user.ID] = &user
//					}
//
//					users := make([]*model.User, len(ids))
//					for i, id := range ids {
//						users[i] = userById[id]
//						i++
//					}
//
//					return users, nil
//				},
//			},
//		})
//		r = r.WithContext(ctx)
//		next.ServeHTTP(w, r)
//	})
//}
