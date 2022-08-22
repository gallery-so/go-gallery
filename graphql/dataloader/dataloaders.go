//go:generate go run github.com/vektah/dataloaden UserLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc.User
//go:generate go run github.com/vektah/dataloaden UsersLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/sqlc.User
//go:generate go run github.com/vektah/dataloaden UserLoaderByAddress github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc.User
//go:generate go run github.com/vektah/dataloaden UserLoaderByString string github.com/mikeydub/go-gallery/db/sqlc.User
//go:generate go run github.com/vektah/dataloaden UsersLoaderByString string []github.com/mikeydub/go-gallery/db/sqlc.User
//go:generate go run github.com/vektah/dataloaden GalleryLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc.Gallery
//go:generate go run github.com/vektah/dataloaden GalleriesLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/sqlc.Gallery
//go:generate go run github.com/vektah/dataloaden CollectionLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc.Collection
//go:generate go run github.com/vektah/dataloaden CollectionsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/sqlc.Collection
//go:generate go run github.com/vektah/dataloaden MembershipLoaderById github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc.Membership
//go:generate go run github.com/vektah/dataloaden WalletLoaderById github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc.Wallet
//go:generate go run github.com/vektah/dataloaden WalletLoaderByChainAddress github.com/mikeydub/go-gallery/service/persist.ChainAddress github.com/mikeydub/go-gallery/db/sqlc.Wallet
//go:generate go run github.com/vektah/dataloaden WalletsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/sqlc.Wallet
//go:generate go run github.com/vektah/dataloaden TokenLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc.Token
//go:generate go run github.com/vektah/dataloaden TokensLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/sqlc.Token
//go:generate go run github.com/vektah/dataloaden ContractLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc.Contract
//go:generate go run github.com/vektah/dataloaden ContractsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/sqlc.Contract
//go:generate go run github.com/vektah/dataloaden ContractLoaderByChainAddress github.com/mikeydub/go-gallery/service/persist.ChainAddress github.com/mikeydub/go-gallery/db/sqlc.Contract
//go:generate go run github.com/vektah/dataloaden GlobalFeedLoader github.com/mikeydub/go-gallery/db/sqlc.GetGlobalFeedViewBatchParams []github.com/mikeydub/go-gallery/db/sqlc.FeedEvent
//go:generate go run github.com/vektah/dataloaden UserFeedLoader github.com/mikeydub/go-gallery/db/sqlc.GetUserFeedViewBatchParams []github.com/mikeydub/go-gallery/db/sqlc.FeedEvent
//go:generate go run github.com/vektah/dataloaden EventLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc.FeedEvent

package dataloader

import (
	"context"
	"database/sql"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/service/persist"
)

const defaultMaxBatchOne = 100 // Default for queries that return a single result
const defaultMaxBatchMany = 10 // Default for queries that return many results
const defaultWaitTime = 2 * time.Millisecond

// Loaders will cache and batch lookups. They are short-lived and should never persist beyond
// a single request, nor should they be shared between requests (since the data returned is
// relative to the current request context, including the user and their auth status).
type Loaders struct {

	// Every entry here must have a corresponding entry in the Clear___Caches methods below

	UserByUserId             UserLoaderByID
	UserByUsername           UserLoaderByString
	UsersWithTrait           UsersLoaderByString
	GalleryByGalleryId       GalleryLoaderByID
	GalleryByCollectionId    GalleryLoaderByID
	GalleriesByUserId        GalleriesLoaderByID
	CollectionByCollectionId CollectionLoaderByID
	CollectionsByGalleryId   CollectionsLoaderByID
	MembershipByMembershipId MembershipLoaderById
	WalletByWalletId         WalletLoaderById
	WalletsByUserID          WalletsLoaderByUserID
	WalletByChainAddress     WalletLoaderByChainAddress
	TokenByTokenID           TokenLoaderByID
	TokensByCollectionID     TokensLoaderByID
	TokensByWalletID         TokensLoaderByID
	TokensByUserID           TokensLoaderByID
	NewTokensByFeedEventID   TokensLoaderByID
	ContractByContractId     ContractLoaderByID
	ContractsByUserID        ContractsLoaderByID
	ContractByChainAddress   ContractLoaderByChainAddress
	FollowersByUserId        UsersLoaderByID
	FollowingByUserId        UsersLoaderByID
	GlobalFeed               GlobalFeedLoader
	FeedByUserId             UserFeedLoader
	EventByEventId           EventLoaderByID
}

func NewLoaders(ctx context.Context, q *sqlc.Queries) *Loaders {
	loaders := &Loaders{}

	loaders.UserByUserId = UserLoaderByID{
		maxBatch: defaultMaxBatchOne,
		wait:     defaultWaitTime,
		fetch:    loadUserByUserId(ctx, loaders, q),
	}

	loaders.UserByUsername = UserLoaderByString{
		maxBatch: defaultMaxBatchOne,
		wait:     defaultWaitTime,
		fetch:    loadUserByUsername(ctx, loaders, q),
	}

	loaders.UsersWithTrait = UsersLoaderByString{
		maxBatch: defaultMaxBatchMany,
		wait:     defaultWaitTime,
		fetch:    loadUsersWithTrait(ctx, loaders, q),
	}

	loaders.GalleryByGalleryId = GalleryLoaderByID{
		maxBatch: defaultMaxBatchOne,
		wait:     defaultWaitTime,
		fetch:    loadGalleryByGalleryId(ctx, loaders, q),
	}

	loaders.GalleryByCollectionId = GalleryLoaderByID{
		maxBatch: defaultMaxBatchOne,
		wait:     defaultWaitTime,
		fetch:    loadGalleryByCollectionId(ctx, loaders, q),
	}

	loaders.GalleriesByUserId = GalleriesLoaderByID{
		maxBatch: defaultMaxBatchMany,
		wait:     defaultWaitTime,
		fetch:    loadGalleriesByUserId(ctx, loaders, q),
	}

	loaders.CollectionByCollectionId = CollectionLoaderByID{
		maxBatch: defaultMaxBatchOne,
		wait:     defaultWaitTime,
		fetch:    loadCollectionByCollectionId(ctx, loaders, q),
	}

	loaders.CollectionsByGalleryId = CollectionsLoaderByID{
		maxBatch: defaultMaxBatchMany,
		wait:     defaultWaitTime,
		fetch:    loadCollectionsByGalleryId(ctx, loaders, q),
	}

	loaders.MembershipByMembershipId = MembershipLoaderById{
		maxBatch: defaultMaxBatchOne,
		wait:     defaultWaitTime,
		fetch:    loadMembershipByMembershipId(ctx, loaders, q),
	}

	loaders.WalletByWalletId = WalletLoaderById{
		maxBatch: defaultMaxBatchOne,
		wait:     defaultWaitTime,
		fetch:    loadWalletByWalletId(ctx, loaders, q),
	}

	loaders.WalletsByUserID = WalletsLoaderByUserID{
		maxBatch: defaultMaxBatchMany,
		wait:     defaultWaitTime,
		fetch:    loadWalletsByUserId(ctx, loaders, q),
	}

	loaders.WalletByChainAddress = WalletLoaderByChainAddress{
		maxBatch: defaultMaxBatchOne,
		wait:     defaultWaitTime,
		fetch:    loadWalletByChainAddress(ctx, loaders, q),
	}

	loaders.FollowersByUserId = UsersLoaderByID{
		maxBatch: defaultMaxBatchMany,
		wait:     defaultWaitTime,
		fetch:    loadFollowersByUserId(ctx, loaders, q),
	}

	loaders.FollowingByUserId = UsersLoaderByID{
		maxBatch: defaultMaxBatchMany,
		wait:     defaultWaitTime,
		fetch:    loadFollowingByUserId(ctx, loaders, q),
	}

	loaders.TokenByTokenID = TokenLoaderByID{
		maxBatch: defaultMaxBatchOne,
		wait:     defaultWaitTime,
		fetch:    loadTokenByTokenID(ctx, loaders, q),
	}

	loaders.TokensByCollectionID = TokensLoaderByID{
		maxBatch: defaultMaxBatchMany,
		wait:     defaultWaitTime,
		fetch:    loadTokensByCollectionID(ctx, loaders, q),
	}

	loaders.TokensByWalletID = TokensLoaderByID{
		maxBatch: defaultMaxBatchMany,
		wait:     defaultWaitTime,
		fetch:    loadTokensByWalletID(ctx, loaders, q),
	}

	loaders.TokensByUserID = TokensLoaderByID{
		maxBatch: defaultMaxBatchMany,
		wait:     defaultWaitTime,
		fetch:    loadTokensByUserID(ctx, loaders, q),
	}

	loaders.NewTokensByFeedEventID = TokensLoaderByID{
		maxBatch: defaultMaxBatchMany,
		wait:     defaultWaitTime,
		fetch:    loadNewTokensByFeedEventID(ctx, loaders, q),
	}

	loaders.ContractByContractId = ContractLoaderByID{
		maxBatch: defaultMaxBatchOne,
		wait:     defaultWaitTime,
		fetch:    loadContractByContractID(ctx, loaders, q),
	}

	loaders.ContractsByUserID = ContractsLoaderByID{
		maxBatch: defaultMaxBatchMany,
		wait:     defaultWaitTime,
		fetch:    loadContractsByUserID(ctx, loaders, q),
	}

	loaders.EventByEventId = EventLoaderByID{
		maxBatch: defaultMaxBatchOne,
		wait:     defaultWaitTime,
		fetch:    loadEventById(ctx, loaders, q),
	}

	loaders.FeedByUserId = UserFeedLoader{
		maxBatch: defaultMaxBatchMany,
		wait:     defaultWaitTime,
		fetch:    loadUserFeed(ctx, loaders, q),
	}

	loaders.GlobalFeed = GlobalFeedLoader{
		maxBatch: defaultMaxBatchMany,
		wait:     defaultWaitTime,
		fetch:    loadGlobalFeed(ctx, loaders, q),
	}

	return loaders
}

// These are pretty verbose and repetitive; hopefully generics make this cleaner in the future

func (l *Loaders) ClearAllCaches() {
	l.ClearUserCaches()
	l.ClearGalleryCaches()
	l.ClearCollectionCaches()
	l.ClearTokenCaches()
	l.ClearMembershipCaches()
	l.ClearFollowCaches()
	l.ClearWalletCaches()
	l.ClearContractCaches()
	l.ClearFeedCaches()
}

func (l *Loaders) ClearUserCaches() {
	l.UserByUserId.mu.Lock()
	l.UserByUserId.cache = nil
	l.UserByUserId.mu.Unlock()

	l.UserByUsername.mu.Lock()
	l.UserByUsername.cache = nil
	l.UserByUsername.mu.Unlock()
}

func (l *Loaders) ClearGalleryCaches() {
	l.GalleryByGalleryId.mu.Lock()
	l.GalleryByGalleryId.cache = nil
	l.GalleryByGalleryId.mu.Unlock()

	l.GalleryByCollectionId.mu.Lock()
	l.GalleryByCollectionId.cache = nil
	l.GalleryByCollectionId.mu.Unlock()

	l.GalleriesByUserId.mu.Lock()
	l.GalleriesByUserId.cache = nil
	l.GalleriesByUserId.mu.Unlock()
}

func (l *Loaders) ClearCollectionCaches() {
	l.CollectionByCollectionId.mu.Lock()
	l.CollectionByCollectionId.cache = nil
	l.CollectionByCollectionId.mu.Unlock()

	l.CollectionsByGalleryId.mu.Lock()
	l.CollectionsByGalleryId.cache = nil
	l.CollectionsByGalleryId.mu.Unlock()
}

func (l *Loaders) ClearTokenCaches() {
	l.TokenByTokenID.mu.Lock()
	l.TokenByTokenID.cache = nil
	l.TokenByTokenID.mu.Unlock()

	l.TokensByCollectionID.mu.Lock()
	l.TokensByCollectionID.cache = nil
	l.TokensByCollectionID.mu.Unlock()

	l.TokensByWalletID.mu.Lock()
	l.TokensByWalletID.cache = nil
	l.TokensByWalletID.mu.Unlock()

	l.TokensByUserID.mu.Lock()
	l.TokensByUserID.cache = nil
	l.TokensByUserID.mu.Unlock()
}

func (l *Loaders) ClearMembershipCaches() {
	l.MembershipByMembershipId.mu.Lock()
	l.MembershipByMembershipId.cache = nil
	l.MembershipByMembershipId.mu.Unlock()
}

func (l *Loaders) ClearFollowCaches() {
	l.FollowersByUserId.mu.Lock()
	l.FollowersByUserId.cache = nil
	l.FollowersByUserId.mu.Unlock()

	l.FollowingByUserId.mu.Lock()
	l.FollowingByUserId.cache = nil
	l.FollowingByUserId.mu.Unlock()
}

func (l *Loaders) ClearWalletCaches() {
	l.WalletByWalletId.mu.Lock()
	l.WalletByWalletId.cache = nil
	l.WalletByWalletId.mu.Unlock()

	l.WalletByChainAddress.mu.Lock()
	l.WalletByChainAddress.cache = nil
	l.WalletByChainAddress.mu.Unlock()

	l.WalletsByUserID.mu.Lock()
	l.WalletsByUserID.cache = nil
	l.WalletsByUserID.mu.Unlock()
}

func (l *Loaders) ClearContractCaches() {
	l.ContractByContractId.mu.Lock()
	l.ContractByContractId.cache = nil
	l.ContractByContractId.mu.Unlock()

	l.ContractByChainAddress.mu.Lock()
	l.ContractByChainAddress.cache = nil
	l.ContractByChainAddress.mu.Unlock()
}

func (l *Loaders) ClearFeedCaches() {
	l.EventByEventId.mu.Lock()
	l.EventByEventId.cache = nil
	l.EventByEventId.mu.Unlock()

	l.FeedByUserId.mu.Lock()
	l.FeedByUserId.cache = nil
	l.FeedByUserId.mu.Unlock()

	l.GlobalFeed.mu.Lock()
	l.GlobalFeed.cache = nil
	l.GlobalFeed.mu.Unlock()
}

func loadUserByUserId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([]sqlc.User, []error) {
	return func(userIds []persist.DBID) ([]sqlc.User, []error) {
		users := make([]sqlc.User, len(userIds))
		errors := make([]error, len(userIds))

		b := q.GetUserByIdBatch(ctx, userIds)
		defer b.Close()

		b.QueryRow(func(i int, user sqlc.User, err error) {
			if err == pgx.ErrNoRows {
				err = persist.ErrUserNotFound{UserID: userIds[i]}
			}

			// Add results to other loaders' caches
			if err == nil {
				loaders.UserByUsername.Prime(user.Username.String, user)
			}

			users[i], errors[i] = user, err
		})

		return users, errors
	}
}

func loadUserByUsername(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]string) ([]sqlc.User, []error) {
	return func(usernames []string) ([]sqlc.User, []error) {
		users := make([]sqlc.User, len(usernames))
		errors := make([]error, len(usernames))

		b := q.GetUserByUsernameBatch(ctx, usernames)
		defer b.Close()

		b.QueryRow(func(i int, user sqlc.User, err error) {
			if err == pgx.ErrNoRows {
				err = persist.ErrUserNotFound{Username: usernames[i]}
			}

			// Add results to other loaders' caches
			if err == nil {
				loaders.UserByUserId.Prime(user.ID, user)
			}

			users[i], errors[i] = user, err
		})

		return users, errors
	}
}

func loadUsersWithTrait(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]string) ([][]sqlc.User, []error) {
	return func(trait []string) ([][]sqlc.User, []error) {
		users := make([][]sqlc.User, len(trait))
		errors := make([]error, len(trait))

		b := q.GetUsersWithTraitBatch(ctx, trait)
		defer b.Close()

		b.Query(func(i int, user []sqlc.User, err error) {

			users[i], errors[i] = user, err

			// Add results to other loaders' caches
			if err == nil {
				for _, u := range user {
					loaders.UserByUserId.Prime(u.ID, u)
				}
			}

		})

		return users, errors
	}
}

func loadGalleryByGalleryId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([]sqlc.Gallery, []error) {
	return func(galleryIds []persist.DBID) ([]sqlc.Gallery, []error) {
		galleries := make([]sqlc.Gallery, len(galleryIds))
		errors := make([]error, len(galleryIds))

		b := q.GetGalleryByIdBatch(ctx, galleryIds)
		defer b.Close()

		b.QueryRow(func(i int, g sqlc.Gallery, err error) {
			galleries[i] = g
			errors[i] = err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrGalleryNotFoundByID{ID: galleryIds[i]}
			}

			// Add results to other loaders' caches
			if errors[i] == nil {
				for _, collection := range galleries[i].Collections {
					loaders.GalleryByCollectionId.Prime(collection, galleries[i])
				}
			}
		})

		return galleries, errors
	}
}

func loadGalleryByCollectionId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([]sqlc.Gallery, []error) {
	return func(collectionIds []persist.DBID) ([]sqlc.Gallery, []error) {
		galleries := make([]sqlc.Gallery, len(collectionIds))
		errors := make([]error, len(collectionIds))

		b := q.GetGalleryByCollectionIdBatch(ctx, collectionIds)
		defer b.Close()

		b.QueryRow(func(i int, g sqlc.Gallery, err error) {
			galleries[i] = g
			errors[i] = err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrGalleryNotFoundByCollectionID{ID: collectionIds[i]}
			}

			// Add results to other loaders' caches
			if errors[i] == nil {
				loaders.GalleryByGalleryId.Prime(galleries[i].ID, galleries[i])
			}
		})

		return galleries, errors
	}
}

func loadGalleriesByUserId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([][]sqlc.Gallery, []error) {
	return func(userIds []persist.DBID) ([][]sqlc.Gallery, []error) {
		galleries := make([][]sqlc.Gallery, len(userIds))
		errors := make([]error, len(userIds))

		b := q.GetGalleriesByUserIdBatch(ctx, userIds)
		defer b.Close()

		b.Query(func(i int, g []sqlc.Gallery, err error) {
			galleries[i] = g
			errors[i] = err

			// Add results to other loaders' caches
			if errors[i] == nil {
				for _, gallery := range galleries[i] {
					loaders.GalleryByGalleryId.Prime(gallery.ID, gallery)
					for _, collection := range gallery.Collections {
						loaders.GalleryByCollectionId.Prime(collection, gallery)
					}
				}
			}
		})

		return galleries, errors
	}
}

func loadCollectionByCollectionId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([]sqlc.Collection, []error) {
	return func(collectionIds []persist.DBID) ([]sqlc.Collection, []error) {
		collections := make([]sqlc.Collection, len(collectionIds))
		errors := make([]error, len(collectionIds))

		b := q.GetCollectionByIdBatch(ctx, collectionIds)
		defer b.Close()

		b.QueryRow(func(i int, c sqlc.Collection, err error) {
			collections[i] = c
			errors[i] = err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrCollectionNotFoundByID{ID: collectionIds[i]}
			}
		})

		return collections, errors
	}
}

func loadCollectionsByGalleryId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([][]sqlc.Collection, []error) {
	return func(galleryIds []persist.DBID) ([][]sqlc.Collection, []error) {
		collections := make([][]sqlc.Collection, len(galleryIds))
		errors := make([]error, len(galleryIds))

		b := q.GetCollectionsByGalleryIdBatch(ctx, galleryIds)
		defer b.Close()

		b.Query(func(i int, c []sqlc.Collection, err error) {
			collections[i] = c
			errors[i] = err

			// Add results to the CollectionByCollectionId loader's cache
			if errors[i] == nil {
				for _, collection := range collections[i] {
					loaders.CollectionByCollectionId.Prime(collection.ID, collection)
				}
			}
		})

		return collections, errors
	}
}

func loadMembershipByMembershipId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([]sqlc.Membership, []error) {
	return func(membershipIds []persist.DBID) ([]sqlc.Membership, []error) {
		memberships := make([]sqlc.Membership, len(membershipIds))
		errors := make([]error, len(membershipIds))

		b := q.GetMembershipByMembershipIdBatch(ctx, membershipIds)
		defer b.Close()

		b.QueryRow(func(i int, m sqlc.Membership, err error) {
			memberships[i] = m
			errors[i] = err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrMembershipNotFoundByID{ID: membershipIds[i]}
			}
		})

		return memberships, errors
	}
}
func loadWalletByWalletId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([]sqlc.Wallet, []error) {
	return func(walletIds []persist.DBID) ([]sqlc.Wallet, []error) {
		wallets := make([]sqlc.Wallet, len(walletIds))
		errors := make([]error, len(walletIds))

		b := q.GetWalletByIDBatch(ctx, walletIds)
		defer b.Close()

		b.QueryRow(func(i int, wallet sqlc.Wallet, err error) {
			// TODO err for not found by ID

			// Add results to other loaders' caches
			if err == nil {
				loaders.WalletByChainAddress.Prime(persist.NewChainAddress(wallet.Address, persist.Chain(wallet.Chain.Int32)), wallet)
			}

			wallets[i], errors[i] = wallet, err
		})

		return wallets, errors
	}
}

func loadWalletsByUserId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([][]sqlc.Wallet, []error) {
	return func(userIds []persist.DBID) ([][]sqlc.Wallet, []error) {
		wallets := make([][]sqlc.Wallet, len(userIds))
		errors := make([]error, len(userIds))

		b := q.GetWalletsByUserIDBatch(ctx, userIds)
		defer b.Close()

		b.Query(func(i int, w []sqlc.Wallet, err error) {
			// TODO err for not found by user ID
			wallets[i], errors[i] = w, err

			// Add results to other loaders' caches
			if errors[i] == nil {
				for _, wallet := range wallets[i] {
					loaders.WalletByWalletId.Prime(wallet.ID, wallet)
					loaders.WalletByChainAddress.Prime(persist.NewChainAddress(wallet.Address, persist.Chain(wallet.Chain.Int32)), wallet)
				}
			}
		})

		return wallets, errors
	}
}

func loadWalletByChainAddress(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.ChainAddress) ([]sqlc.Wallet, []error) {
	return func(chainAddresses []persist.ChainAddress) ([]sqlc.Wallet, []error) {
		wallets := make([]sqlc.Wallet, len(chainAddresses))
		errors := make([]error, len(chainAddresses))

		sqlChainAddress := make([]sqlc.GetWalletByChainAddressBatchParams, len(chainAddresses))
		for i, chainAddress := range chainAddresses {
			sqlChainAddress[i] = sqlc.GetWalletByChainAddressBatchParams{
				Address: chainAddress.Address(),
				Chain:   sql.NullInt32{Int32: int32(chainAddress.Chain()), Valid: true},
			}
		}

		b := q.GetWalletByChainAddressBatch(ctx, sqlChainAddress)
		defer b.Close()

		b.QueryRow(func(i int, wallet sqlc.Wallet, err error) {
			if err == pgx.ErrNoRows {
				err = persist.ErrWalletNotFound{ChainAddress: chainAddresses[i]}
			}

			// Add results to other loaders' caches
			if err == nil {
				loaders.WalletByWalletId.Prime(wallet.ID, wallet)
			}

			wallets[i], errors[i] = wallet, err
		})

		return wallets, errors
	}
}

func loadFollowersByUserId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([][]sqlc.User, []error) {
	return func(userIds []persist.DBID) ([][]sqlc.User, []error) {
		followers := make([][]sqlc.User, len(userIds))
		errors := make([]error, len(followers))

		b := q.GetFollowersByUserIdBatch(ctx, userIds)
		defer b.Close()

		b.Query(func(i int, u []sqlc.User, err error) {
			followers[i] = u
			errors[i] = err

			// Add results to other loaders' caches
			if err == nil {
				for _, user := range followers[i] {
					loaders.UserByUsername.Prime(user.Username.String, user)
					loaders.UserByUserId.Prime(user.ID, user)
				}
			}
		})

		return followers, errors
	}
}

func loadFollowingByUserId(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([][]sqlc.User, []error) {
	return func(userIds []persist.DBID) ([][]sqlc.User, []error) {
		following := make([][]sqlc.User, len(userIds))
		errors := make([]error, len(following))

		b := q.GetFollowingByUserIdBatch(ctx, userIds)
		defer b.Close()

		b.Query(func(i int, u []sqlc.User, err error) {
			following[i] = u
			errors[i] = err

			// Add results to other loaders' caches
			if err == nil {
				for _, user := range following[i] {
					loaders.UserByUsername.Prime(user.Username.String, user)
					loaders.UserByUserId.Prime(user.ID, user)
				}
			}
		})

		return following, errors
	}
}

func loadTokenByTokenID(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([]sqlc.Token, []error) {
	return func(tokenIDs []persist.DBID) ([]sqlc.Token, []error) {
		tokens := make([]sqlc.Token, len(tokenIDs))
		errors := make([]error, len(tokenIDs))

		b := q.GetTokenByIdBatch(ctx, tokenIDs)
		defer b.Close()

		b.QueryRow(func(i int, t sqlc.Token, err error) {
			tokens[i], errors[i] = t, err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrTokenNotFoundByID{ID: tokenIDs[i]}
			}
		})

		return tokens, errors
	}
}

func loadTokensByCollectionID(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([][]sqlc.Token, []error) {
	return func(collectionIDs []persist.DBID) ([][]sqlc.Token, []error) {
		tokens := make([][]sqlc.Token, len(collectionIDs))
		errors := make([]error, len(collectionIDs))

		b := q.GetTokensByCollectionIdBatch(ctx, collectionIDs)
		defer b.Close()

		b.Query(func(i int, t []sqlc.Token, err error) {
			tokens[i], errors[i] = t, err

			// Add results to the TokenByTokenID loader's cache
			if errors[i] == nil {
				for _, token := range tokens[i] {
					loaders.TokenByTokenID.Prime(token.ID, token)
				}
			}
		})

		return tokens, errors
	}
}

func loadTokensByWalletID(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([][]sqlc.Token, []error) {
	return func(walletIds []persist.DBID) ([][]sqlc.Token, []error) {
		tokens := make([][]sqlc.Token, len(walletIds))
		errors := make([]error, len(walletIds))

		convertedIds := make([]persist.DBIDList, len(walletIds))
		for i, id := range walletIds {
			convertedIds[i] = persist.DBIDList{id}
		}

		b := q.GetTokensByWalletIdsBatch(ctx, convertedIds)
		defer b.Close()

		b.Query(func(i int, t []sqlc.Token, err error) {
			tokens[i], errors[i] = t, err

			// Add results to the TokenByTokenID loader's cache
			if errors[i] == nil {
				for _, token := range tokens[i] {
					loaders.TokenByTokenID.Prime(token.ID, token)
				}
			}
		})

		return tokens, errors
	}
}

func loadTokensByUserID(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([][]sqlc.Token, []error) {
	return func(userIDs []persist.DBID) ([][]sqlc.Token, []error) {
		tokens := make([][]sqlc.Token, len(userIDs))
		errors := make([]error, len(userIDs))

		b := q.GetTokensByUserIdBatch(ctx, userIDs)
		defer b.Close()

		b.Query(func(i int, t []sqlc.Token, err error) {
			tokens[i], errors[i] = t, err

			// Add results to the TokenByTokenID loader's cache
			if errors[i] == nil {
				for _, token := range tokens[i] {
					loaders.TokenByTokenID.Prime(token.ID, token)
				}
			}
		})

		return tokens, errors
	}
}

func loadNewTokensByFeedEventID(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([][]sqlc.Token, []error) {
	return func(tokenIDs []persist.DBID) ([][]sqlc.Token, []error) {
		tokens := make([][]sqlc.Token, len(tokenIDs))
		errors := make([]error, len(tokenIDs))

		b := q.GetNewTokensByFeedEventIdBatch(ctx, tokenIDs)
		defer b.Close()

		b.Query(func(i int, t []sqlc.Token, err error) {
			tokens[i], errors[i] = t, err

			// Add results to the TokenByTokenID loader's cache
			if errors[i] == nil {
				for _, token := range tokens[i] {
					loaders.TokenByTokenID.Prime(token.ID, token)
				}
			}
		})

		return tokens, errors
	}
}

func loadContractByContractID(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([]sqlc.Contract, []error) {
	return func(contractIDs []persist.DBID) ([]sqlc.Contract, []error) {
		contracts := make([]sqlc.Contract, len(contractIDs))
		errors := make([]error, len(contractIDs))

		b := q.GetContractByIDBatch(ctx, contractIDs)
		defer b.Close()

		b.QueryRow(func(i int, t sqlc.Contract, err error) {
			contracts[i], errors[i] = t, err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrContractNotFoundByID{ID: contractIDs[i]}
			}
		})

		return contracts, errors
	}
}

func loadContractsByUserID(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([][]sqlc.Contract, []error) {
	return func(contractIDs []persist.DBID) ([][]sqlc.Contract, []error) {
		contracts := make([][]sqlc.Contract, len(contractIDs))
		errors := make([]error, len(contractIDs))

		b := q.GetContractsByUserIDBatch(ctx, contractIDs)
		defer b.Close()

		b.Query(func(i int, c []sqlc.Contract, err error) {
			contracts[i], errors[i] = c, err

			// Add results to the ContractByContractId loader's cache
			if errors[i] == nil {
				for _, contract := range contracts[i] {
					loaders.ContractByContractId.Prime(contract.ID, contract)
				}
			}

		})

		return contracts, errors
	}
}

func loadEventById(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]persist.DBID) ([]sqlc.FeedEvent, []error) {
	return func(eventIds []persist.DBID) ([]sqlc.FeedEvent, []error) {
		events := make([]sqlc.FeedEvent, len(eventIds))
		errors := make([]error, len(eventIds))

		b := q.GetEventByIdBatch(ctx, eventIds)
		defer b.Close()

		b.QueryRow(func(i int, p sqlc.FeedEvent, err error) {
			events[i] = p
			errors[i] = err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrFeedEventNotFoundByID{ID: eventIds[i]}
			}
		})

		return events, errors
	}
}

func loadUserFeed(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]sqlc.GetUserFeedViewBatchParams) ([][]sqlc.FeedEvent, []error) {
	return func(params []sqlc.GetUserFeedViewBatchParams) ([][]sqlc.FeedEvent, []error) {
		events := make([][]sqlc.FeedEvent, len(params))
		errors := make([]error, len(params))

		b := q.GetUserFeedViewBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, evts []sqlc.FeedEvent, err error) {
			events[i] = evts
			errors[i] = err

			// Add results to the EventById loader's cache
			if errors[i] == nil {
				for _, p := range evts {
					loaders.EventByEventId.Prime(p.ID, p)
				}
			}
		})

		return events, errors
	}
}

func loadGlobalFeed(ctx context.Context, loaders *Loaders, q *sqlc.Queries) func([]sqlc.GetGlobalFeedViewBatchParams) ([][]sqlc.FeedEvent, []error) {
	return func(params []sqlc.GetGlobalFeedViewBatchParams) ([][]sqlc.FeedEvent, []error) {
		events := make([][]sqlc.FeedEvent, len(params))
		errors := make([]error, len(params))

		b := q.GetGlobalFeedViewBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, evts []sqlc.FeedEvent, err error) {
			events[i] = evts
			errors[i] = err

			// Add results to the EventById loader's cache
			if errors[i] == nil {
				for _, p := range evts {
					loaders.EventByEventId.Prime(p.ID, p)
				}
			}
		})

		return events, errors
	}
}
