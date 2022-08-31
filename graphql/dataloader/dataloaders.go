<<<<<<< HEAD
//go:generate go run github.com/vektah/dataloaden UserLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.User
//go:generate go run github.com/vektah/dataloaden UsersLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.User
//go:generate go run github.com/vektah/dataloaden UserLoaderByAddress github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.User
//go:generate go run github.com/vektah/dataloaden UserLoaderByString string github.com/mikeydub/go-gallery/db/gen/coredb.User
//go:generate go run github.com/vektah/dataloaden UsersLoaderByString string []github.com/mikeydub/go-gallery/db/gen/coredb.User
//go:generate go run github.com/vektah/dataloaden GalleryLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Gallery
//go:generate go run github.com/vektah/dataloaden GalleriesLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Gallery
//go:generate go run github.com/vektah/dataloaden CollectionLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Collection
//go:generate go run github.com/vektah/dataloaden CollectionsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Collection
//go:generate go run github.com/vektah/dataloaden MembershipLoaderById github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Membership
//go:generate go run github.com/vektah/dataloaden WalletLoaderById github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Wallet
//go:generate go run github.com/vektah/dataloaden WalletLoaderByChainAddress github.com/mikeydub/go-gallery/service/persist.ChainAddress github.com/mikeydub/go-gallery/db/gen/coredb.Wallet
//go:generate go run github.com/vektah/dataloaden WalletsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Wallet
//go:generate go run github.com/vektah/dataloaden TokenLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Token
//go:generate go run github.com/vektah/dataloaden TokensLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Token
//go:generate go run github.com/vektah/dataloaden TokensLoaderByIDAndChain github.com/mikeydub/go-gallery/graphql/dataloader.IDAndChain []github.com/mikeydub/go-gallery/db/gen/coredb.Token
//go:generate go run github.com/vektah/dataloaden ContractLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Contract
//go:generate go run github.com/vektah/dataloaden ContractsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Contract
//go:generate go run github.com/vektah/dataloaden ContractLoaderByChainAddress github.com/mikeydub/go-gallery/service/persist.ChainAddress github.com/mikeydub/go-gallery/db/gen/coredb.Contract
//go:generate go run github.com/vektah/dataloaden GlobalFeedLoader github.com/mikeydub/go-gallery/db/gen/coredb.GetGlobalFeedViewBatchParams []github.com/mikeydub/go-gallery/db/gen/coredb.FeedEvent
//go:generate go run github.com/vektah/dataloaden UserFeedLoader github.com/mikeydub/go-gallery/db/gen/coredb.GetUserFeedViewBatchParams []github.com/mikeydub/go-gallery/db/gen/coredb.FeedEvent
//go:generate go run github.com/vektah/dataloaden EventLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.FeedEvent
//go:generate go run github.com/vektah/dataloaden AdmireLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Admire
//go:generate go run github.com/vektah/dataloaden AdmiresLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Admire
//go:generate go run github.com/vektah/dataloaden CommentLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Comment
//go:generate go run github.com/vektah/dataloaden CommentsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Comment
=======
//go:generate go run github.com/vektah/dataloaden UserLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc/coregen.User
//go:generate go run github.com/vektah/dataloaden UsersLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/sqlc/coregen.User
//go:generate go run github.com/vektah/dataloaden UserLoaderByAddress github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc/coregen.User
//go:generate go run github.com/vektah/dataloaden UserLoaderByString string github.com/mikeydub/go-gallery/db/sqlc/coregen.User
//go:generate go run github.com/vektah/dataloaden UsersLoaderByString string []github.com/mikeydub/go-gallery/db/sqlc/coregen.User
//go:generate go run github.com/vektah/dataloaden GalleryLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc/coregen.Gallery
//go:generate go run github.com/vektah/dataloaden GalleriesLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/sqlc/coregen.Gallery
//go:generate go run github.com/vektah/dataloaden CollectionLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc/coregen.Collection
//go:generate go run github.com/vektah/dataloaden CollectionsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/sqlc/coregen.Collection
//go:generate go run github.com/vektah/dataloaden MembershipLoaderById github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc/coregen.Membership
//go:generate go run github.com/vektah/dataloaden WalletLoaderById github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc/coregen.Wallet
//go:generate go run github.com/vektah/dataloaden WalletLoaderByChainAddress github.com/mikeydub/go-gallery/service/persist.ChainAddress github.com/mikeydub/go-gallery/db/sqlc/coregen.Wallet
//go:generate go run github.com/vektah/dataloaden WalletsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/sqlc/coregen.Wallet
//go:generate go run github.com/vektah/dataloaden TokenLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc/coregen.Token
//go:generate go run github.com/vektah/dataloaden TokensLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/sqlc/coregen.Token
//go:generate go run github.com/vektah/dataloaden TokensLoaderByIDAndChain github.com/mikeydub/go-gallery/graphql/dataloader.IDAndChain []github.com/mikeydub/go-gallery/db/sqlc/coregen.Token
//go:generate go run github.com/vektah/dataloaden ContractLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc/coregen.Contract
//go:generate go run github.com/vektah/dataloaden ContractsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/sqlc/coregen.Contract
//go:generate go run github.com/vektah/dataloaden ContractLoaderByChainAddress github.com/mikeydub/go-gallery/service/persist.ChainAddress github.com/mikeydub/go-gallery/db/sqlc/coregen.Contract
//go:generate go run github.com/vektah/dataloaden GlobalFeedLoader github.com/mikeydub/go-gallery/db/sqlc/coregen.GetGlobalFeedViewBatchParams []github.com/mikeydub/go-gallery/db/sqlc/coregen.FeedEvent
//go:generate go run github.com/vektah/dataloaden UserFeedLoader github.com/mikeydub/go-gallery/db/sqlc/coregen.GetUserFeedViewBatchParams []github.com/mikeydub/go-gallery/db/sqlc/coregen.FeedEvent
//go:generate go run github.com/vektah/dataloaden EventLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc/coregen.FeedEvent
//go:generate go run github.com/vektah/dataloaden AdmireLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc/coregen.Admire
//go:generate go run github.com/vektah/dataloaden AdmiresLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/sqlc/coregen.Admire
//go:generate go run github.com/vektah/dataloaden CommentLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/sqlc/coregen.Comment
//go:generate go run github.com/vektah/dataloaden CommentsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/sqlc/coregen.Comment

>>>>>>> a4e9c3f (Add indexer models)
package dataloader

import (
	"context"
	"database/sql"
	"time"

	"github.com/jackc/pgx/v4"
<<<<<<< HEAD
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
=======
	sqlc "github.com/mikeydub/go-gallery/db/sqlc/coregen"
>>>>>>> a4e9c3f (Add indexer models)
	"github.com/mikeydub/go-gallery/service/persist"
)

const defaultMaxBatchOne = 100 // Default for queries that return a single result
const defaultMaxBatchMany = 10 // Default for queries that return many results
const defaultWaitTime = 2 * time.Millisecond

type IDAndChain struct {
	ID    persist.DBID
	Chain persist.Chain
}

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
	WalletsByUserID          WalletsLoaderByID
	WalletByChainAddress     WalletLoaderByChainAddress
	TokenByTokenID           TokenLoaderByID
	TokensByCollectionID     TokensLoaderByID
	TokensByWalletID         TokensLoaderByID
	TokensByUserID           TokensLoaderByID
	TokensByUserIDAndChain   TokensLoaderByIDAndChain
	NewTokensByFeedEventID   TokensLoaderByID
	ContractByContractId     ContractLoaderByID
	ContractsByUserID        ContractsLoaderByID
	ContractByChainAddress   ContractLoaderByChainAddress
	FollowersByUserId        UsersLoaderByID
	FollowingByUserId        UsersLoaderByID
	GlobalFeed               GlobalFeedLoader
	FeedByUserId             UserFeedLoader
	EventByEventId           EventLoaderByID
	AdmireByAdmireId         AdmireLoaderByID
	AdmiresByFeedEventId     AdmiresLoaderByID
	CommentByCommentId       CommentLoaderByID
	CommentsByFeedEventId    CommentsLoaderByID
}

func NewLoaders(ctx context.Context, q *db.Queries) *Loaders {
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

	loaders.WalletsByUserID = WalletsLoaderByID{
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

	loaders.TokensByUserIDAndChain = TokensLoaderByIDAndChain{
		maxBatch: defaultMaxBatchMany,
		wait:     defaultWaitTime,
		fetch:    loadTokensByUserIDAndChain(ctx, loaders, q),
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

	loaders.ContractByChainAddress = ContractLoaderByChainAddress{
		maxBatch: defaultMaxBatchOne,
		wait:     defaultWaitTime,
		fetch:    loadContractByChainAddress(ctx, loaders, q),
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

	loaders.AdmireByAdmireId = AdmireLoaderByID{
		maxBatch: defaultMaxBatchOne,
		wait:     defaultWaitTime,
		fetch:    loadAdmireById(ctx, loaders, q),
	}

	loaders.AdmiresByFeedEventId = AdmiresLoaderByID{
		maxBatch: defaultMaxBatchMany,
		wait:     defaultWaitTime,
		fetch:    loadAdmiresByFeedEventId(ctx, loaders, q),
	}

	loaders.CommentByCommentId = CommentLoaderByID{
		maxBatch: defaultMaxBatchOne,
		wait:     defaultWaitTime,
		fetch:    loadCommentById(ctx, loaders, q),
	}

	loaders.CommentsByFeedEventId = CommentsLoaderByID{
		maxBatch: defaultMaxBatchMany,
		wait:     defaultWaitTime,
		fetch:    loadCommentsByFeedEventId(ctx, loaders, q),
	}

	return loaders
}

// fillErrors fills a slice of errors with the specified error. Useful for batched lookups where
// a single top-level error may need to be returned for each request in the batch.
func fillErrors(errors []error, err error) {
	for i := 0; i < len(errors); i++ {
		errors[i] = err
	}
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

func loadUserByUserId(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([]db.User, []error) {
	return func(userIds []persist.DBID) ([]db.User, []error) {
		users := make([]db.User, len(userIds))
		errors := make([]error, len(userIds))

		b := q.GetUserByIdBatch(ctx, userIds)
		defer b.Close()

		b.QueryRow(func(i int, user db.User, err error) {
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

func loadUserByUsername(ctx context.Context, loaders *Loaders, q *db.Queries) func([]string) ([]db.User, []error) {
	return func(usernames []string) ([]db.User, []error) {
		users := make([]db.User, len(usernames))
		errors := make([]error, len(usernames))

		b := q.GetUserByUsernameBatch(ctx, usernames)
		defer b.Close()

		b.QueryRow(func(i int, user db.User, err error) {
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

func loadUsersWithTrait(ctx context.Context, loaders *Loaders, q *db.Queries) func([]string) ([][]db.User, []error) {
	return func(trait []string) ([][]db.User, []error) {
		users := make([][]db.User, len(trait))
		errors := make([]error, len(trait))

		b := q.GetUsersWithTraitBatch(ctx, trait)
		defer b.Close()

		b.Query(func(i int, user []db.User, err error) {

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

func loadGalleryByGalleryId(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([]db.Gallery, []error) {
	return func(galleryIds []persist.DBID) ([]db.Gallery, []error) {
		galleries := make([]db.Gallery, len(galleryIds))
		errors := make([]error, len(galleryIds))

		b := q.GetGalleryByIdBatch(ctx, galleryIds)
		defer b.Close()

		b.QueryRow(func(i int, g db.Gallery, err error) {
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

func loadGalleryByCollectionId(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([]db.Gallery, []error) {
	return func(collectionIds []persist.DBID) ([]db.Gallery, []error) {
		galleries := make([]db.Gallery, len(collectionIds))
		errors := make([]error, len(collectionIds))

		b := q.GetGalleryByCollectionIdBatch(ctx, collectionIds)
		defer b.Close()

		b.QueryRow(func(i int, g db.Gallery, err error) {
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

func loadGalleriesByUserId(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([][]db.Gallery, []error) {
	return func(userIds []persist.DBID) ([][]db.Gallery, []error) {
		galleries := make([][]db.Gallery, len(userIds))
		errors := make([]error, len(userIds))

		b := q.GetGalleriesByUserIdBatch(ctx, userIds)
		defer b.Close()

		b.Query(func(i int, g []db.Gallery, err error) {
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

func loadCollectionByCollectionId(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([]db.Collection, []error) {
	return func(collectionIds []persist.DBID) ([]db.Collection, []error) {
		collections := make([]db.Collection, len(collectionIds))
		errors := make([]error, len(collectionIds))

		b := q.GetCollectionByIdBatch(ctx, collectionIds)
		defer b.Close()

		b.QueryRow(func(i int, c db.Collection, err error) {
			collections[i] = c
			errors[i] = err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrCollectionNotFoundByID{ID: collectionIds[i]}
			}
		})

		return collections, errors
	}
}

func loadCollectionsByGalleryId(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([][]db.Collection, []error) {
	return func(galleryIds []persist.DBID) ([][]db.Collection, []error) {
		collections := make([][]db.Collection, len(galleryIds))
		errors := make([]error, len(galleryIds))

		b := q.GetCollectionsByGalleryIdBatch(ctx, galleryIds)
		defer b.Close()

		b.Query(func(i int, c []db.Collection, err error) {
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

func loadMembershipByMembershipId(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([]db.Membership, []error) {
	return func(membershipIds []persist.DBID) ([]db.Membership, []error) {
		memberships := make([]db.Membership, len(membershipIds))
		errors := make([]error, len(membershipIds))

		b := q.GetMembershipByMembershipIdBatch(ctx, membershipIds)
		defer b.Close()

		b.QueryRow(func(i int, m db.Membership, err error) {
			memberships[i] = m
			errors[i] = err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrMembershipNotFoundByID{ID: membershipIds[i]}
			}
		})

		return memberships, errors
	}
}
func loadWalletByWalletId(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([]db.Wallet, []error) {
	return func(walletIds []persist.DBID) ([]db.Wallet, []error) {
		wallets := make([]db.Wallet, len(walletIds))
		errors := make([]error, len(walletIds))

		b := q.GetWalletByIDBatch(ctx, walletIds)
		defer b.Close()

		b.QueryRow(func(i int, wallet db.Wallet, err error) {
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

func loadWalletsByUserId(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([][]db.Wallet, []error) {
	return func(userIds []persist.DBID) ([][]db.Wallet, []error) {
		wallets := make([][]db.Wallet, len(userIds))
		errors := make([]error, len(userIds))

		b := q.GetWalletsByUserIDBatch(ctx, userIds)
		defer b.Close()

		b.Query(func(i int, w []db.Wallet, err error) {
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

func loadWalletByChainAddress(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.ChainAddress) ([]db.Wallet, []error) {
	return func(chainAddresses []persist.ChainAddress) ([]db.Wallet, []error) {
		wallets := make([]db.Wallet, len(chainAddresses))
		errors := make([]error, len(chainAddresses))

		sqlChainAddress := make([]db.GetWalletByChainAddressBatchParams, len(chainAddresses))
		for i, chainAddress := range chainAddresses {
			sqlChainAddress[i] = db.GetWalletByChainAddressBatchParams{
				Address: chainAddress.Address(),
				Chain:   sql.NullInt32{Int32: int32(chainAddress.Chain()), Valid: true},
			}
		}

		b := q.GetWalletByChainAddressBatch(ctx, sqlChainAddress)
		defer b.Close()

		b.QueryRow(func(i int, wallet db.Wallet, err error) {
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

func loadFollowersByUserId(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([][]db.User, []error) {
	return func(userIds []persist.DBID) ([][]db.User, []error) {
		followers := make([][]db.User, len(userIds))
		errors := make([]error, len(followers))

		b := q.GetFollowersByUserIdBatch(ctx, userIds)
		defer b.Close()

		b.Query(func(i int, u []db.User, err error) {
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

func loadFollowingByUserId(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([][]db.User, []error) {
	return func(userIds []persist.DBID) ([][]db.User, []error) {
		following := make([][]db.User, len(userIds))
		errors := make([]error, len(following))

		b := q.GetFollowingByUserIdBatch(ctx, userIds)
		defer b.Close()

		b.Query(func(i int, u []db.User, err error) {
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

func loadTokenByTokenID(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([]db.Token, []error) {
	return func(tokenIDs []persist.DBID) ([]db.Token, []error) {
		tokens := make([]db.Token, len(tokenIDs))
		errors := make([]error, len(tokenIDs))

		b := q.GetTokenByIdBatch(ctx, tokenIDs)
		defer b.Close()

		b.QueryRow(func(i int, t db.Token, err error) {
			tokens[i], errors[i] = t, err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrTokenNotFoundByID{ID: tokenIDs[i]}
			}
		})

		return tokens, errors
	}
}

func loadTokensByCollectionID(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([][]db.Token, []error) {
	return func(collectionIDs []persist.DBID) ([][]db.Token, []error) {
		tokens := make([][]db.Token, len(collectionIDs))
		errors := make([]error, len(collectionIDs))

		b := q.GetTokensByCollectionIdBatch(ctx, collectionIDs)
		defer b.Close()

		b.Query(func(i int, t []db.Token, err error) {
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

func loadTokensByWalletID(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([][]db.Token, []error) {
	return func(walletIds []persist.DBID) ([][]db.Token, []error) {
		tokens := make([][]db.Token, len(walletIds))
		errors := make([]error, len(walletIds))

		convertedIds := make([]persist.DBIDList, len(walletIds))
		for i, id := range walletIds {
			convertedIds[i] = persist.DBIDList{id}
		}

		b := q.GetTokensByWalletIdsBatch(ctx, convertedIds)
		defer b.Close()

		b.Query(func(i int, t []db.Token, err error) {
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

func loadTokensByUserID(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([][]db.Token, []error) {
	return func(userIDs []persist.DBID) ([][]db.Token, []error) {
		tokens := make([][]db.Token, len(userIDs))
		errors := make([]error, len(userIDs))

		b := q.GetTokensByUserIdBatch(ctx, userIDs)
		defer b.Close()

		b.Query(func(i int, t []db.Token, err error) {
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

func loadTokensByUserIDAndChain(ctx context.Context, loaders *Loaders, q *db.Queries) func([]IDAndChain) ([][]db.Token, []error) {
	return func(userIDsAndChains []IDAndChain) ([][]db.Token, []error) {
		tokens := make([][]db.Token, len(userIDsAndChains))
		errors := make([]error, len(userIDsAndChains))

		params := make([]db.GetTokensByUserIdAndChainBatchParams, len(userIDsAndChains))
		for i, userIDAndChain := range userIDsAndChains {
			params[i] = db.GetTokensByUserIdAndChainBatchParams{
				OwnerUserID: userIDAndChain.ID,
				Chain:       sql.NullInt32{Int32: int32(userIDAndChain.Chain), Valid: true},
			}
		}

		b := q.GetTokensByUserIdAndChainBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, t []db.Token, err error) {
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

func loadNewTokensByFeedEventID(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([][]db.Token, []error) {
	return func(tokenIDs []persist.DBID) ([][]db.Token, []error) {
		tokens := make([][]db.Token, len(tokenIDs))
		errors := make([]error, len(tokenIDs))

		b := q.GetNewTokensByFeedEventIdBatch(ctx, tokenIDs)
		defer b.Close()

		b.Query(func(i int, t []db.Token, err error) {
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

func loadContractByContractID(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([]db.Contract, []error) {
	return func(contractIDs []persist.DBID) ([]db.Contract, []error) {
		contracts := make([]db.Contract, len(contractIDs))
		errors := make([]error, len(contractIDs))

		rows, err := q.GetContractsByIDs(ctx, contractIDs)
		if err != nil {
			fillErrors(errors, err)
			return contracts, errors
		}

		contractsByID := make(map[persist.DBID]db.Contract)
		for _, row := range rows {
			contractsByID[row.ID] = row
		}

		for i, id := range contractIDs {
			if contract, ok := contractsByID[id]; ok {
				contracts[i] = contract
			} else {
				errors[i] = persist.ErrContractNotFoundByID{ID: id}
			}
		}

		return contracts, errors
	}
}

func loadContractByChainAddress(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.ChainAddress) ([]db.Contract, []error) {
	return func(chainAddresses []persist.ChainAddress) ([]db.Contract, []error) {
		contracts := make([]db.Contract, len(chainAddresses))
		errors := make([]error, len(chainAddresses))

		asParams := make([]db.GetContractByChainAddressBatchParams, len(chainAddresses))
		for i, chainAddress := range chainAddresses {
			asParams[i] = db.GetContractByChainAddressBatchParams{
				Chain:   sql.NullInt32{Int32: int32(chainAddress.Chain()), Valid: true},
				Address: chainAddress.Address(),
			}
		}
		b := q.GetContractByChainAddressBatch(ctx, asParams)
		defer b.Close()

		b.QueryRow(func(i int, t db.Contract, err error) {
			contracts[i], errors[i] = t, err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrGalleryContractNotFound{Address: chainAddresses[i].Address(), Chain: chainAddresses[i].Chain()}
			}
		})

		return contracts, errors
	}
}

func loadContractsByUserID(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([][]db.Contract, []error) {
	return func(contractIDs []persist.DBID) ([][]db.Contract, []error) {
		contracts := make([][]db.Contract, len(contractIDs))
		errors := make([]error, len(contractIDs))

		b := q.GetContractsByUserIDBatch(ctx, contractIDs)
		defer b.Close()

		b.Query(func(i int, c []db.Contract, err error) {
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

func loadEventById(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([]db.FeedEvent, []error) {
	return func(eventIds []persist.DBID) ([]db.FeedEvent, []error) {
		events := make([]db.FeedEvent, len(eventIds))
		errors := make([]error, len(eventIds))

		b := q.GetEventByIdBatch(ctx, eventIds)
		defer b.Close()

		b.QueryRow(func(i int, p db.FeedEvent, err error) {
			events[i] = p
			errors[i] = err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrFeedEventNotFoundByID{ID: eventIds[i]}
			}
		})

		return events, errors
	}
}

func loadUserFeed(ctx context.Context, loaders *Loaders, q *db.Queries) func([]db.GetUserFeedViewBatchParams) ([][]db.FeedEvent, []error) {
	return func(params []db.GetUserFeedViewBatchParams) ([][]db.FeedEvent, []error) {
		events := make([][]db.FeedEvent, len(params))
		errors := make([]error, len(params))

		b := q.GetUserFeedViewBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, evts []db.FeedEvent, err error) {
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

func loadGlobalFeed(ctx context.Context, loaders *Loaders, q *db.Queries) func([]db.GetGlobalFeedViewBatchParams) ([][]db.FeedEvent, []error) {
	return func(params []db.GetGlobalFeedViewBatchParams) ([][]db.FeedEvent, []error) {
		events := make([][]db.FeedEvent, len(params))
		errors := make([]error, len(params))

		b := q.GetGlobalFeedViewBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, evts []db.FeedEvent, err error) {
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

func loadAdmireById(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([]db.Admire, []error) {
	return func(admireIds []persist.DBID) ([]db.Admire, []error) {
		admires := make([]db.Admire, len(admireIds))
		errors := make([]error, len(admireIds))

		b := q.GetAdmireByAdmireIDBatch(ctx, admireIds)
		defer b.Close()

		b.QueryRow(func(i int, a db.Admire, err error) {
			admires[i] = a
			errors[i] = err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrAdmireNotFound{ID: admireIds[i]}
			}
		})

		return admires, errors
	}
}

func loadAdmiresByFeedEventId(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([][]db.Admire, []error) {
	return func(ids []persist.DBID) ([][]db.Admire, []error) {
		admires := make([][]db.Admire, len(ids))
		errors := make([]error, len(ids))

		b := q.GetAdmiresByFeedEventIDBatch(ctx, ids)
		defer b.Close()

		b.Query(func(i int, admrs []db.Admire, err error) {
			admires[i] = admrs
			errors[i] = err

			// Add results to the AdmireByAdmireId loader's cache
			if errors[i] == nil {
				for _, a := range admrs {
					loaders.AdmireByAdmireId.Prime(a.ID, a)
				}
			}
		})

		return admires, errors
	}
}

func loadCommentById(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([]db.Comment, []error) {
	return func(commentIds []persist.DBID) ([]db.Comment, []error) {
		comments := make([]db.Comment, len(commentIds))
		errors := make([]error, len(commentIds))

		b := q.GetCommentByCommentIDBatch(ctx, commentIds)
		defer b.Close()

		b.QueryRow(func(i int, c db.Comment, err error) {
			comments[i] = c
			errors[i] = err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrCommentNotFound{ID: commentIds[i]}
			}
		})

		return comments, errors
	}
}

func loadCommentsByFeedEventId(ctx context.Context, loaders *Loaders, q *db.Queries) func([]persist.DBID) ([][]db.Comment, []error) {
	return func(ids []persist.DBID) ([][]db.Comment, []error) {
		comments := make([][]db.Comment, len(ids))
		errors := make([]error, len(ids))

		b := q.GetCommentsByFeedEventIDBatch(ctx, ids)
		defer b.Close()

		b.Query(func(i int, cmts []db.Comment, err error) {
			comments[i] = cmts
			errors[i] = err

			// Add results to the CommentById loader's cache
			if errors[i] == nil {
				for _, c := range cmts {
					loaders.CommentByCommentId.Prime(c.ID, c)
				}
			}
		})

		return comments, errors
	}
}
