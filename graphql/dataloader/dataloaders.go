//go:generate go run github.com/gallery-so/dataloaden UserLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.User
//go:generate go run github.com/gallery-so/dataloaden UsersLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.User
//go:generate go run github.com/gallery-so/dataloaden UserLoaderByAddress github.com/mikeydub/go-gallery/db/gen/coredb.GetUserByAddressBatchParams github.com/mikeydub/go-gallery/db/gen/coredb.User
//go:generate go run github.com/gallery-so/dataloaden UserLoaderByString string github.com/mikeydub/go-gallery/db/gen/coredb.User
//go:generate go run github.com/gallery-so/dataloaden UsersLoaderByString string []github.com/mikeydub/go-gallery/db/gen/coredb.User
//go:generate go run github.com/gallery-so/dataloaden UsersLoaderByContractID github.com/mikeydub/go-gallery/db/gen/coredb.GetOwnersByContractIdBatchPaginateParams []github.com/mikeydub/go-gallery/db/gen/coredb.User
//go:generate go run github.com/gallery-so/dataloaden GalleryLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Gallery
//go:generate go run github.com/gallery-so/dataloaden GalleriesLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Gallery
//go:generate go run github.com/gallery-so/dataloaden CollectionLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Collection
//go:generate go run github.com/gallery-so/dataloaden CollectionsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Collection
//go:generate go run github.com/gallery-so/dataloaden MembershipLoaderById github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Membership
//go:generate go run github.com/gallery-so/dataloaden WalletLoaderById github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Wallet
//go:generate go run github.com/gallery-so/dataloaden WalletLoaderByChainAddress github.com/mikeydub/go-gallery/service/persist.ChainAddress github.com/mikeydub/go-gallery/db/gen/coredb.Wallet
//go:generate go run github.com/gallery-so/dataloaden WalletsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Wallet
//go:generate go run github.com/gallery-so/dataloaden TokenLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Token
//go:generate go run github.com/gallery-so/dataloaden TokenLoaderByHolderIDContractAddressAndTokenID github.com/mikeydub/go-gallery/db/gen/coredb.GetTokenByHolderIdContractAddressAndTokenIdBatchParams github.com/mikeydub/go-gallery/db/gen/coredb.Token
//go:generate go run github.com/gallery-so/dataloaden TokensLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Token
//go:generate go run github.com/gallery-so/dataloaden TokensLoaderByIDAndLimit github.com/mikeydub/go-gallery/graphql/dataloader.IDAndLimit []github.com/mikeydub/go-gallery/db/gen/coredb.Token
//go:generate go run github.com/gallery-so/dataloaden TokensLoaderByIDAndChain github.com/mikeydub/go-gallery/graphql/dataloader.IDAndChain []github.com/mikeydub/go-gallery/db/gen/coredb.Token
//go:generate go run github.com/gallery-so/dataloaden ContractLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Contract
//go:generate go run github.com/gallery-so/dataloaden ContractsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Contract
//go:generate go run github.com/gallery-so/dataloaden ContractsLoaderByCreatorID github.com/mikeydub/go-gallery/db/gen/coredb.GetCreatedContractsBatchPaginateParams []github.com/mikeydub/go-gallery/db/gen/coredb.Contract
//go:generate go run github.com/gallery-so/dataloaden ContractsLoaderByParentID github.com/mikeydub/go-gallery/db/gen/coredb.GetChildContractsByParentIDBatchPaginateParams []github.com/mikeydub/go-gallery/db/gen/coredb.Contract
//go:generate go run github.com/gallery-so/dataloaden ContractLoaderByChainAddress github.com/mikeydub/go-gallery/service/persist.ChainAddress github.com/mikeydub/go-gallery/db/gen/coredb.Contract
//go:generate go run github.com/gallery-so/dataloaden EventLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.FeedEvent
//go:generate go run github.com/gallery-so/dataloaden PostLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Post
//go:generate go run github.com/gallery-so/dataloaden PostsPaginatedLoaderByContractID github.com/mikeydub/go-gallery/db/gen/coredb.PaginatePostsByContractIDParams []github.com/mikeydub/go-gallery/db/gen/coredb.Post
//go:generate go run github.com/gallery-so/dataloaden AdmireLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Admire
//go:generate go run github.com/gallery-so/dataloaden AdmiresLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Admire
//go:generate go run github.com/gallery-so/dataloaden CommentLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Comment
//go:generate go run github.com/gallery-so/dataloaden CommentsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Comment
//go:generate go run github.com/gallery-so/dataloaden NotificationLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Notification
//go:generate go run github.com/gallery-so/dataloaden NotificationsLoaderByUserID github.com/mikeydub/go-gallery/db/gen/coredb.GetUserNotificationsBatchParams []github.com/mikeydub/go-gallery/db/gen/coredb.Notification
//go:generate go run github.com/gallery-so/dataloaden FeedEventCommentsLoader github.com/mikeydub/go-gallery/db/gen/coredb.PaginateCommentsByFeedEventIDBatchParams []github.com/mikeydub/go-gallery/db/gen/coredb.Comment
//go:generate go run github.com/gallery-so/dataloaden FeedEventAdmiresLoader github.com/mikeydub/go-gallery/db/gen/coredb.PaginateAdmiresByFeedEventIDBatchParams []github.com/mikeydub/go-gallery/db/gen/coredb.Admire
//go:generate go run github.com/gallery-so/dataloaden PostCommentsLoader github.com/mikeydub/go-gallery/db/gen/coredb.PaginateCommentsByPostIDBatchParams []github.com/mikeydub/go-gallery/db/gen/coredb.Comment
//go:generate go run github.com/gallery-so/dataloaden RepliesLoader github.com/mikeydub/go-gallery/db/gen/coredb.PaginateRepliesByCommentIDBatchParams []github.com/mikeydub/go-gallery/db/gen/coredb.Comment
//go:generate go run github.com/gallery-so/dataloaden PostAdmiresLoader github.com/mikeydub/go-gallery/db/gen/coredb.PaginateAdmiresByPostIDBatchParams []github.com/mikeydub/go-gallery/db/gen/coredb.Admire
//go:generate go run github.com/gallery-so/dataloaden FeedEventInteractionsLoader github.com/mikeydub/go-gallery/db/gen/coredb.PaginateInteractionsByFeedEventIDBatchParams []github.com/mikeydub/go-gallery/db/gen/coredb.PaginateInteractionsByFeedEventIDBatchRow
//go:generate go run github.com/gallery-so/dataloaden FeedEventInteractionCountLoader github.com/mikeydub/go-gallery/db/gen/coredb.CountInteractionsByFeedEventIDBatchParams []github.com/mikeydub/go-gallery/db/gen/coredb.CountInteractionsByFeedEventIDBatchRow
//go:generate go run github.com/gallery-so/dataloaden PostInteractionsLoader github.com/mikeydub/go-gallery/db/gen/coredb.PaginateInteractionsByPostIDBatchParams []github.com/mikeydub/go-gallery/db/gen/coredb.PaginateInteractionsByPostIDBatchRow
//go:generate go run github.com/gallery-so/dataloaden PostInteractionCountLoader github.com/mikeydub/go-gallery/db/gen/coredb.CountInteractionsByPostIDBatchParams []github.com/mikeydub/go-gallery/db/gen/coredb.CountInteractionsByPostIDBatchRow
//go:generate go run github.com/gallery-so/dataloaden IntLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID int
//go:generate go run github.com/gallery-so/dataloaden AdmireLoaderByActorAndFeedEvent github.com/mikeydub/go-gallery/db/gen/coredb.GetAdmireByActorIDAndFeedEventIDParams github.com/mikeydub/go-gallery/db/gen/coredb.Admire
//go:generate go run github.com/gallery-so/dataloaden AdmireLoaderByActorAndPost github.com/mikeydub/go-gallery/db/gen/coredb.GetAdmireByActorIDAndPostIDParams github.com/mikeydub/go-gallery/db/gen/coredb.Admire
//go:generate go run github.com/gallery-so/dataloaden SharedFollowersLoaderByIDs github.com/mikeydub/go-gallery/db/gen/coredb.GetSharedFollowersBatchPaginateParams []github.com/mikeydub/go-gallery/db/gen/coredb.GetSharedFollowersBatchPaginateRow
//go:generate go run github.com/gallery-so/dataloaden SharedContractsLoaderByIDs github.com/mikeydub/go-gallery/db/gen/coredb.GetSharedContractsBatchPaginateParams []github.com/mikeydub/go-gallery/db/gen/coredb.GetSharedContractsBatchPaginateRow
//go:generate go run github.com/gallery-so/dataloaden MediaLoaderByTokenID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.TokenMedia
//go:generate go run github.com/gallery-so/dataloaden ContractCreatorLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.ContractCreator
//go:generate go run github.com/gallery-so/dataloaden TokensLoaderByUserIDAndFilters github.com/mikeydub/go-gallery/db/gen/coredb.GetTokensByUserIdBatchParams []github.com/mikeydub/go-gallery/db/gen/coredb.Token
//go:generate go run github.com/gallery-so/dataloaden ProfileImageLoaderByID github.com/mikeydub/go-gallery/db/gen/coredb.GetProfileImageByIDParams github.com/mikeydub/go-gallery/db/gen/coredb.ProfileImage
//go:generate go run github.com/gallery-so/dataloaden GalleryTokenPreviewsByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.TokenMedia

package dataloader

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/mikeydub/go-gallery/util"

	"github.com/jackc/pgx/v4"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

type IDAndChain struct {
	ID    persist.DBID
	Chain persist.Chain
}

type IDAndLimit struct {
	ID    persist.DBID
	Limit *int
}

// Loaders will cache and batch lookups. They are short-lived and should never persist beyond
// a single request, nor should they be shared between requests (since the data returned is
// relative to the current request context, including the user and their auth status).
type Loaders struct {
	UserByUserID                             *UserLoaderByID
	UserByUsername                           *UserLoaderByString
	UserByAddress                            *UserLoaderByAddress
	UsersWithTrait                           *UsersLoaderByString
	GalleryByGalleryID                       *GalleryLoaderByID
	GalleryByCollectionID                    *GalleryLoaderByID
	GalleriesByUserID                        *GalleriesLoaderByID
	CollectionByCollectionID                 *CollectionLoaderByID
	CollectionsByGalleryID                   *CollectionsLoaderByID
	MembershipByMembershipID                 *MembershipLoaderById
	WalletByWalletID                         *WalletLoaderById
	WalletsByUserID                          *WalletsLoaderByID
	WalletByChainAddress                     *WalletLoaderByChainAddress
	TokenByTokenID                           *TokenLoaderByID
	TokenByHolderIDContractAddressAndTokenID *TokenLoaderByHolderIDContractAddressAndTokenID
	TokensByContractID                       *TokensLoaderByID
	TokensByCollectionID                     *TokensLoaderByIDAndLimit
	TokensByWalletID                         *TokensLoaderByID
	TokensByUserID                           *TokensLoaderByUserIDAndFilters
	TokensByUserIDAndChain                   *TokensLoaderByIDAndChain
	NewTokensByFeedEventID                   *TokensLoaderByID
	OwnerByTokenID                           *UserLoaderByID
	ContractByContractID                     *ContractLoaderByID
	ContractsLoaderByCreatorID               *ContractsLoaderByCreatorID
	ContractsLoaderByParentID                *ContractsLoaderByParentID
	ContractsByUserID                        *ContractsLoaderByID
	ContractByChainAddress                   *ContractLoaderByChainAddress
	FollowersByUserID                        *UsersLoaderByID
	FollowingByUserID                        *UsersLoaderByID
	SharedFollowersByUserIDs                 *SharedFollowersLoaderByIDs
	SharedContractsByUserIDs                 *SharedContractsLoaderByIDs
	FeedEventByFeedEventID                   *EventLoaderByID
	PostByPostID                             *PostLoaderByID
	PostsPaginatedByContractID               *PostsPaginatedLoaderByContractID
	AdmireByAdmireID                         *AdmireLoaderByID
	AdmireCountByFeedEventID                 *IntLoaderByID
	AdmiresByFeedEventID                     *FeedEventAdmiresLoader
	AdmireCountByPostID                      *IntLoaderByID
	AdmiresByPostID                          *PostAdmiresLoader
	CommentByCommentID                       *CommentLoaderByID
	CommentCountByFeedEventID                *IntLoaderByID
	CommentsByFeedEventID                    *FeedEventCommentsLoader
	CommentCountByPostID                     *IntLoaderByID
	CommentsByPostID                         *PostCommentsLoader
	RepliesByCommentID                       *RepliesLoader
	RepliesCountByCommentID                  *IntLoaderByID
	InteractionCountByFeedEventID            *FeedEventInteractionCountLoader
	InteractionsByFeedEventID                *FeedEventInteractionsLoader
	InteractionCountByPostID                 *PostInteractionCountLoader
	InteractionsByPostID                     *PostInteractionsLoader
	NotificationByID                         *NotificationLoaderByID
	NotificationsByUserID                    *NotificationsLoaderByUserID
	ContractsDisplayedByUserID               *ContractsLoaderByID
	OwnersByContractID                       *UsersLoaderByContractID
	AdmireByActorIDAndFeedEventID            *AdmireLoaderByActorAndFeedEvent
	AdmireByActorIDAndPostID                 *AdmireLoaderByActorAndPost
	MediaByTokenID                           *MediaLoaderByTokenID
	ContractCreatorByContractID              *ContractCreatorLoaderByID
	ProfileImageByID                         *ProfileImageLoaderByID
	GalleryTokenPreviewsByID                 *GalleryTokenPreviewsByID
}

func NewLoaders(ctx context.Context, q *db.Queries, disableCaching bool) *Loaders {
	subscriptionRegistry := make([]interface{}, 0)
	mutexRegistry := make([]*sync.Mutex, 0)
	defaults := defaultSettings(ctx, disableCaching, &subscriptionRegistry, &mutexRegistry)

	//---------------------------------------------------------------------------------------------------
	// HOW TO ADD A NEW DATALOADER
	//---------------------------------------------------------------------------------------------------
	// 1) If you need a new loader type, add it to the top of the file and use the "go generate" command
	//    to generate it. The convention is to name your loader <ValueType>LoaderBy<KeyType>, where
	//    <ValueType> should be plural if your loader returns a slice. Note that a loader type can be
	//    used by multiple dataloaders: UserLoaderByID is the correct generated type for both a
	//    "UserByUserID" dataloader and a "UserByGalleryID" dataloader.
	//
	// 2) Add your dataloader to the Loaders struct above
	//
	// 3) Initialize your loader below. Dataloaders that don't return slices can subscribe to automatic
	//    cache priming by specifying an AutoCacheWithKey function (which should return the key to use
	//    when caching). If your dataloader needs to cache a single value with multiple keys (e.g. a
	//    GalleryByCollectionID wants to cache a single Gallery by many collection IDs), you can use
	//    AutoCacheWithKeys instead. When other dataloaders return the type you've subscribed to, your
	//    dataloader will automatically cache those results.
	//
	//    Note: dataloaders that return slices can't subscribe to automatic caching, because it's
	//          unlikely that the grouping of results returned by one dataloader will make sense for
	//          another. E.g. the results of TokensByWalletID have little to do with the results
	//			of TokensByCollectionID, even though they both return slices of Tokens.
	//
	// 4) The "defaults" struct has sufficient settings for most use cases, but if you need to override
	//	  any default settings, all NewLoader methods accept these option args:
	//		- withMaxBatch(batchSize int)		<-- set the max batch size for a loader
	//		- withMaxWait(wait time.Duration)	<-- set the max wait time for a loader
	//		- withPublishResults(publish bool)  <-- whether this loader should publish its results for
	//  											other loaders to subscribe to and cache
	//---------------------------------------------------------------------------------------------------

	loaders := &Loaders{}

	loaders.UserByUserID = NewUserLoaderByID(defaults, loadUserByUserId(q), UserLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(user db.User) persist.DBID { return user.ID },
	})

	loaders.UserByUsername = NewUserLoaderByString(defaults, loadUserByUsername(q), UserLoaderByStringCacheSubscriptions{
		AutoCacheWithKey: func(user db.User) string { return user.Username.String },
	})

	loaders.UserByAddress = NewUserLoaderByAddress(defaults, loadUserByAddress(q), UserLoaderByAddressCacheSubscriptions{})

	loaders.UsersWithTrait = NewUsersLoaderByString(defaults, loadUsersWithTrait(q))

	loaders.OwnersByContractID = NewUsersLoaderByContractID(defaults, loadOwnersByContractIDs(q))

	loaders.GalleryByGalleryID = NewGalleryLoaderByID(defaults, loadGalleryByGalleryId(q), GalleryLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(gallery db.Gallery) persist.DBID { return gallery.ID },
	})

	loaders.GalleryByCollectionID = NewGalleryLoaderByID(defaults, loadGalleryByCollectionId(q), GalleryLoaderByIDCacheSubscriptions{
		AutoCacheWithKeys: func(gallery db.Gallery) []persist.DBID { return gallery.Collections },
	})

	loaders.GalleriesByUserID = NewGalleriesLoaderByID(defaults, loadGalleriesByUserId(q))

	loaders.CollectionByCollectionID = NewCollectionLoaderByID(defaults, loadCollectionByCollectionId(q), CollectionLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(collection db.Collection) persist.DBID { return collection.ID },
	})

	loaders.CollectionsByGalleryID = NewCollectionsLoaderByID(defaults, loadCollectionsByGalleryId(q))

	loaders.MembershipByMembershipID = NewMembershipLoaderById(defaults, loadMembershipByMembershipId(q), MembershipLoaderByIdCacheSubscriptions{
		AutoCacheWithKey: func(membership db.Membership) persist.DBID { return membership.ID },
	})

	loaders.WalletByWalletID = NewWalletLoaderById(defaults, loadWalletByWalletId(q), WalletLoaderByIdCacheSubscriptions{
		AutoCacheWithKey: func(wallet db.Wallet) persist.DBID { return wallet.ID },
	})

	loaders.WalletsByUserID = NewWalletsLoaderByID(defaults, loadWalletsByUserId(q))

	loaders.WalletByChainAddress = NewWalletLoaderByChainAddress(defaults, loadWalletByChainAddress(q), WalletLoaderByChainAddressCacheSubscriptions{
		AutoCacheWithKey: func(wallet db.Wallet) persist.ChainAddress {
			return persist.NewChainAddress(wallet.Address, wallet.Chain)
		},
	})

	loaders.FollowersByUserID = NewUsersLoaderByID(defaults, loadFollowersByUserId(q))

	loaders.FollowingByUserID = NewUsersLoaderByID(defaults, loadFollowingByUserId(q))

	loaders.SharedFollowersByUserIDs = NewSharedFollowersLoaderByIDs(defaults, loadSharedFollowersByIDs(q))

	loaders.SharedContractsByUserIDs = NewSharedContractsLoaderByIDs(defaults, loadSharedContractsByIDs(q))

	loaders.TokenByTokenID = NewTokenLoaderByID(defaults, loadTokenByTokenID(q), TokenLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(token db.Token) persist.DBID { return token.ID },
	})

	loaders.TokenByHolderIDContractAddressAndTokenID = NewTokenLoaderByHolderIDContractAddressAndTokenID(defaults, loadTokenByHolderIDContractAddressAndTokenID(q), TokenLoaderByHolderIDContractAddressAndTokenIDCacheSubscriptions{})

	loaders.TokensByCollectionID = NewTokensLoaderByIDAndLimit(defaults, loadTokensByCollectionID(q))

	loaders.TokensByWalletID = NewTokensLoaderByID(defaults, loadTokensByWalletID(q))

	loaders.TokensByUserID = NewTokensLoaderByUserIDAndFilters(defaults, loadTokensByUserID(q))

	loaders.TokensByUserIDAndChain = NewTokensLoaderByIDAndChain(defaults, loadTokensByUserIDAndChain(q))

	loaders.TokensByUserIDAndChain = NewTokensLoaderByIDAndChain(defaults, loadTokensByUserIDAndChain(q))

	loaders.OwnerByTokenID = NewUserLoaderByID(defaults, loadOwnerByTokenID(q), UserLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(user db.User) persist.DBID { return user.ID },
	})

	loaders.NewTokensByFeedEventID = NewTokensLoaderByID(defaults, loadNewTokensByFeedEventID(q))

	loaders.ContractByContractID = NewContractLoaderByID(
		settingsWithOptions(ctx, disableCaching, &subscriptionRegistry, &mutexRegistry, withMaxBatchOne(500), withWaitTime(5*time.Millisecond)),
		loadContractByContractID(q),
		ContractLoaderByIDCacheSubscriptions{AutoCacheWithKey: func(contract db.Contract) persist.DBID { return contract.ID }},
	)

	loaders.ContractByChainAddress = NewContractLoaderByChainAddress(defaults, loadContractByChainAddress(q), ContractLoaderByChainAddressCacheSubscriptions{
		AutoCacheWithKey: func(contract db.Contract) persist.ChainAddress {
			return persist.NewChainAddress(contract.Address, contract.Chain)
		},
	})

	loaders.ContractsLoaderByCreatorID = NewContractsLoaderByCreatorID(defaults, loadContractsByCreatorID(q))

	loaders.ContractsLoaderByParentID = NewContractsLoaderByParentID(defaults, loadContractsByParentID(q))

	loaders.FeedEventByFeedEventID = NewEventLoaderByID(defaults, loadEventById(q), EventLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(event db.FeedEvent) persist.DBID { return event.ID },
	})

	loaders.ContractsDisplayedByUserID = NewContractsLoaderByID(defaults, loadContractsDisplayedByUserID(q))

	loaders.PostByPostID = NewPostLoaderByID(defaults, loadPostById(q), PostLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(post db.Post) persist.DBID { return post.ID },
	})

	loaders.PostsPaginatedByContractID = NewPostsPaginatedLoaderByContractID(defaults, loadPostsPaginatedByContractID(q))

	loaders.NotificationsByUserID = NewNotificationsLoaderByUserID(defaults, loadUserNotifications(q))

	loaders.AdmireByAdmireID = NewAdmireLoaderByID(defaults, loadAdmireById(q), AdmireLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(admire db.Admire) persist.DBID { return admire.ID },
	})

	loaders.AdmireCountByFeedEventID = NewIntLoaderByID(defaults, loadAdmireCountByFeedEventID(q), IntLoaderByIDCacheSubscriptions{})

	loaders.AdmireCountByPostID = NewIntLoaderByID(defaults, loadAdmireCountByPostID(q), IntLoaderByIDCacheSubscriptions{})

	loaders.AdmiresByFeedEventID = NewFeedEventAdmiresLoader(defaults, loadAdmiresByFeedEventID(q))

	loaders.AdmiresByPostID = NewPostAdmiresLoader(defaults, loadAdmiresByPostID(q))

	loaders.CommentByCommentID = NewCommentLoaderByID(defaults, loadCommentById(q), CommentLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(comment db.Comment) persist.DBID { return comment.ID },
	})

	loaders.CommentCountByFeedEventID = NewIntLoaderByID(defaults, loadCommentCountByFeedEventID(q), IntLoaderByIDCacheSubscriptions{})

	loaders.CommentCountByPostID = NewIntLoaderByID(defaults, loadCommentCountByPostID(q), IntLoaderByIDCacheSubscriptions{})

	loaders.CommentsByFeedEventID = NewFeedEventCommentsLoader(defaults, loadCommentsByFeedEventID(q))

	loaders.CommentsByPostID = NewPostCommentsLoader(defaults, loadCommentsByPostID(q))

	loaders.RepliesByCommentID = NewRepliesLoader(defaults, loadRepliesByCommentID(q))

	loaders.InteractionCountByFeedEventID = NewFeedEventInteractionCountLoader(defaults, loadInteractionCountByFeedEventID(q))

	loaders.InteractionsByFeedEventID = NewFeedEventInteractionsLoader(defaults, loadInteractionsByFeedEventID(q))

	loaders.InteractionsByPostID = NewPostInteractionsLoader(defaults, loadInteractionsByPostID(q))

	loaders.InteractionCountByPostID = NewPostInteractionCountLoader(defaults, loadInteractionCountByPostID(q))

	loaders.AdmireByActorIDAndFeedEventID = NewAdmireLoaderByActorAndFeedEvent(defaults, loadAdmireByActorIDAndFeedEventID(q), AdmireLoaderByActorAndFeedEventCacheSubscriptions{})

	loaders.AdmireByActorIDAndPostID = NewAdmireLoaderByActorAndPost(defaults, loadAdmireByActorIDAndPostID(q), AdmireLoaderByActorAndPostCacheSubscriptions{})

	loaders.NotificationByID = NewNotificationLoaderByID(defaults, loadNotificationById(q), NotificationLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(notification db.Notification) persist.DBID { return notification.ID },
	})

	loaders.MediaByTokenID = NewMediaLoaderByTokenID(
		settingsWithOptions(ctx, disableCaching, &subscriptionRegistry, &mutexRegistry, withMaxBatchOne(500), withWaitTime(5*time.Millisecond)),
		loadMediaByTokenID(q),
		MediaLoaderByTokenIDCacheSubscriptions{AutoCacheWithKey: func(media db.TokenMedia) persist.DBID { return media.ID }})

	loaders.ContractCreatorByContractID = NewContractCreatorLoaderByID(defaults, loadContractCreatorByContractID(q), ContractCreatorLoaderByIDCacheSubscriptions{})

	loaders.ProfileImageByID = NewProfileImageLoaderByID(defaults, loadProfileImageByID(q), ProfileImageLoaderByIDCacheSubscriptions{})

	loaders.GalleryTokenPreviewsByID = NewGalleryTokenPreviewsByID(defaults, loadGalleryTokenPreviewsByID(q))

	return loaders
}

func loadUserByUserId(q *db.Queries) func(context.Context, []persist.DBID) ([]db.User, []error) {
	return func(ctx context.Context, userIds []persist.DBID) ([]db.User, []error) {
		users := make([]db.User, len(userIds))
		errors := make([]error, len(userIds))

		b := q.GetUserByIdBatch(ctx, userIds)
		defer b.Close()

		b.QueryRow(func(i int, user db.User, err error) {
			if err == pgx.ErrNoRows {
				err = persist.ErrUserNotFound{UserID: userIds[i]}
			}

			users[i], errors[i] = user, err
		})

		return users, errors
	}
}

func loadUserByUsername(q *db.Queries) func(context.Context, []string) ([]db.User, []error) {
	return func(ctx context.Context, usernames []string) ([]db.User, []error) {
		users := make([]db.User, len(usernames))
		errors := make([]error, len(usernames))

		b := q.GetUserByUsernameBatch(ctx, usernames)
		defer b.Close()

		b.QueryRow(func(i int, user db.User, err error) {
			if err == pgx.ErrNoRows {
				err = persist.ErrUserNotFound{Username: usernames[i]}
			}

			users[i], errors[i] = user, err
		})

		return users, errors
	}
}

func loadUserByAddress(q *db.Queries) func(context.Context, []db.GetUserByAddressBatchParams) ([]db.User, []error) {
	return func(ctx context.Context, params []db.GetUserByAddressBatchParams) ([]db.User, []error) {
		users := make([]db.User, len(params))
		errors := make([]error, len(params))

		b := q.GetUserByAddressBatch(ctx, params)
		defer b.Close()

		b.QueryRow(func(i int, user db.User, err error) {
			if err == pgx.ErrNoRows {
				err = persist.ErrUserNotFound{ChainAddress: persist.NewChainAddress(params[i].Address, persist.Chain(params[i].Chain))}
			}

			users[i], errors[i] = user, err
		})

		return users, errors
	}
}

func loadOwnersByContractIDs(q *db.Queries) func(context.Context, []db.GetOwnersByContractIdBatchPaginateParams) ([][]db.User, []error) {
	return func(ctx context.Context, params []db.GetOwnersByContractIdBatchPaginateParams) ([][]db.User, []error) {
		users := make([][]db.User, len(params))
		errors := make([]error, len(params))

		b := q.GetOwnersByContractIdBatchPaginate(ctx, params)
		defer b.Close()

		b.Query(func(i int, user []db.User, err error) {
			users[i], errors[i] = user, err
		})

		return users, errors
	}
}

func loadUsersWithTrait(q *db.Queries) func(context.Context, []string) ([][]db.User, []error) {
	return func(ctx context.Context, trait []string) ([][]db.User, []error) {
		users := make([][]db.User, len(trait))
		errors := make([]error, len(trait))

		b := q.GetUsersWithTraitBatch(ctx, trait)
		defer b.Close()

		b.Query(func(i int, user []db.User, err error) {
			users[i], errors[i] = user, err
		})

		return users, errors
	}
}

func loadGalleryByGalleryId(q *db.Queries) func(context.Context, []persist.DBID) ([]db.Gallery, []error) {
	return func(ctx context.Context, galleryIds []persist.DBID) ([]db.Gallery, []error) {
		galleries := make([]db.Gallery, len(galleryIds))
		errors := make([]error, len(galleryIds))

		b := q.GetGalleryByIdBatch(ctx, galleryIds)
		defer b.Close()

		b.QueryRow(func(i int, g db.Gallery, err error) {
			galleries[i] = g
			errors[i] = err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrGalleryNotFound{ID: galleryIds[i]}
			}
		})

		return galleries, errors
	}
}

func loadGalleryByCollectionId(q *db.Queries) func(context.Context, []persist.DBID) ([]db.Gallery, []error) {
	return func(ctx context.Context, collectionIds []persist.DBID) ([]db.Gallery, []error) {
		galleries := make([]db.Gallery, len(collectionIds))
		errors := make([]error, len(collectionIds))

		b := q.GetGalleryByCollectionIdBatch(ctx, collectionIds)
		defer b.Close()

		b.QueryRow(func(i int, g db.Gallery, err error) {
			galleries[i] = g
			errors[i] = err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrGalleryNotFound{CollectionID: collectionIds[i]}
			}
		})

		return galleries, errors
	}
}

func loadGalleriesByUserId(q *db.Queries) func(context.Context, []persist.DBID) ([][]db.Gallery, []error) {
	return func(ctx context.Context, userIds []persist.DBID) ([][]db.Gallery, []error) {
		galleries := make([][]db.Gallery, len(userIds))
		errors := make([]error, len(userIds))

		b := q.GetGalleriesByUserIdBatch(ctx, userIds)
		defer b.Close()

		b.Query(func(i int, g []db.Gallery, err error) {
			galleries[i] = g
			errors[i] = err
		})

		return galleries, errors
	}
}

func loadNotificationById(q *db.Queries) func(context.Context, []persist.DBID) ([]db.Notification, []error) {
	return func(ctx context.Context, ids []persist.DBID) ([]db.Notification, []error) {
		notifs := make([]db.Notification, len(ids))
		errors := make([]error, len(ids))

		b := q.GetNotificationByIDBatch(ctx, ids)
		defer b.Close()

		b.QueryRow(func(i int, n db.Notification, err error) {
			errors[i] = err
			notifs[i] = n
		})

		return notifs, errors
	}
}

func loadCollectionByCollectionId(q *db.Queries) func(context.Context, []persist.DBID) ([]db.Collection, []error) {
	return func(ctx context.Context, collectionIds []persist.DBID) ([]db.Collection, []error) {
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

func loadCollectionsByGalleryId(q *db.Queries) func(context.Context, []persist.DBID) ([][]db.Collection, []error) {
	return func(ctx context.Context, galleryIds []persist.DBID) ([][]db.Collection, []error) {
		collections := make([][]db.Collection, len(galleryIds))
		errors := make([]error, len(galleryIds))

		b := q.GetCollectionsByGalleryIdBatch(ctx, galleryIds)
		defer b.Close()

		b.Query(func(i int, c []db.Collection, err error) {
			collections[i] = c
			errors[i] = err
		})

		return collections, errors
	}
}

func loadMembershipByMembershipId(q *db.Queries) func(context.Context, []persist.DBID) ([]db.Membership, []error) {
	return func(ctx context.Context, membershipIds []persist.DBID) ([]db.Membership, []error) {
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
func loadWalletByWalletId(q *db.Queries) func(context.Context, []persist.DBID) ([]db.Wallet, []error) {
	return func(ctx context.Context, walletIds []persist.DBID) ([]db.Wallet, []error) {
		wallets := make([]db.Wallet, len(walletIds))
		errors := make([]error, len(walletIds))

		b := q.GetWalletByIDBatch(ctx, walletIds)
		defer b.Close()

		b.QueryRow(func(i int, wallet db.Wallet, err error) {
			// TODO err for not found by ID
			wallets[i], errors[i] = wallet, err
		})

		return wallets, errors
	}
}

func loadWalletsByUserId(q *db.Queries) func(context.Context, []persist.DBID) ([][]db.Wallet, []error) {
	return func(ctx context.Context, userIds []persist.DBID) ([][]db.Wallet, []error) {
		wallets := make([][]db.Wallet, len(userIds))
		errors := make([]error, len(userIds))

		b := q.GetWalletsByUserIDBatch(ctx, userIds)
		defer b.Close()

		b.Query(func(i int, w []db.Wallet, err error) {
			// TODO err for not found by user ID
			wallets[i], errors[i] = w, err
		})

		return wallets, errors
	}
}

func loadWalletByChainAddress(q *db.Queries) func(context.Context, []persist.ChainAddress) ([]db.Wallet, []error) {
	return func(ctx context.Context, chainAddresses []persist.ChainAddress) ([]db.Wallet, []error) {
		wallets := make([]db.Wallet, len(chainAddresses))
		errors := make([]error, len(chainAddresses))

		sqlChainAddress := make([]db.GetWalletByChainAddressBatchParams, len(chainAddresses))
		for i, chainAddress := range chainAddresses {
			sqlChainAddress[i] = db.GetWalletByChainAddressBatchParams{
				Address: chainAddress.Address(),
				Chain:   chainAddress.Chain(),
			}
		}

		b := q.GetWalletByChainAddressBatch(ctx, sqlChainAddress)
		defer b.Close()

		b.QueryRow(func(i int, wallet db.Wallet, err error) {
			if err == pgx.ErrNoRows {
				err = persist.ErrWalletNotFound{ChainAddress: chainAddresses[i]}
			}

			wallets[i], errors[i] = wallet, err
		})

		return wallets, errors
	}
}

func loadFollowersByUserId(q *db.Queries) func(context.Context, []persist.DBID) ([][]db.User, []error) {
	return func(ctx context.Context, userIds []persist.DBID) ([][]db.User, []error) {
		followers := make([][]db.User, len(userIds))
		errors := make([]error, len(followers))

		b := q.GetFollowersByUserIdBatch(ctx, userIds)
		defer b.Close()

		b.Query(func(i int, u []db.User, err error) {
			followers[i] = u
			errors[i] = err
		})

		return followers, errors
	}
}

func loadFollowingByUserId(q *db.Queries) func(context.Context, []persist.DBID) ([][]db.User, []error) {
	return func(ctx context.Context, userIds []persist.DBID) ([][]db.User, []error) {
		following := make([][]db.User, len(userIds))
		errors := make([]error, len(following))

		b := q.GetFollowingByUserIdBatch(ctx, userIds)
		defer b.Close()

		b.Query(func(i int, u []db.User, err error) {
			following[i] = u
			errors[i] = err
		})

		return following, errors
	}
}

func loadSharedFollowersByIDs(q *db.Queries) func(context.Context, []db.GetSharedFollowersBatchPaginateParams) ([][]db.GetSharedFollowersBatchPaginateRow, []error) {
	return func(ctx context.Context, params []db.GetSharedFollowersBatchPaginateParams) ([][]db.GetSharedFollowersBatchPaginateRow, []error) {
		users := make([][]db.GetSharedFollowersBatchPaginateRow, len(params))
		errors := make([]error, len(users))

		b := q.GetSharedFollowersBatchPaginate(ctx, params)
		defer b.Close()

		b.Query(func(i int, u []db.GetSharedFollowersBatchPaginateRow, err error) {
			users[i] = u
			errors[i] = err
		})

		return users, errors
	}
}

func loadSharedContractsByIDs(q *db.Queries) func(context.Context, []db.GetSharedContractsBatchPaginateParams) ([][]db.GetSharedContractsBatchPaginateRow, []error) {
	return func(ctx context.Context, params []db.GetSharedContractsBatchPaginateParams) ([][]db.GetSharedContractsBatchPaginateRow, []error) {
		contracts := make([][]db.GetSharedContractsBatchPaginateRow, len(params))
		errors := make([]error, len(contracts))

		b := q.GetSharedContractsBatchPaginate(ctx, params)
		defer b.Close()

		b.Query(func(i int, c []db.GetSharedContractsBatchPaginateRow, err error) {
			contracts[i] = c
			errors[i] = err
		})

		return contracts, errors
	}
}

func loadTokenByTokenID(q *db.Queries) func(context.Context, []persist.DBID) ([]db.Token, []error) {
	return func(ctx context.Context, tokenIDs []persist.DBID) ([]db.Token, []error) {
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

func loadTokenByHolderIDContractAddressAndTokenID(q *db.Queries) func(context.Context, []db.GetTokenByHolderIdContractAddressAndTokenIdBatchParams) ([]db.Token, []error) {
	return func(ctx context.Context, params []db.GetTokenByHolderIdContractAddressAndTokenIdBatchParams) ([]db.Token, []error) {
		tokens := make([]db.Token, len(params))
		errors := make([]error, len(params))

		b := q.GetTokenByHolderIdContractAddressAndTokenIdBatch(ctx, params)
		defer b.Close()

		b.QueryRow(func(i int, t db.Token, err error) {
			tokens[i], errors[i] = t, err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrTokenNotFoundByHolderIdentifiers{
					HolderID:        params[i].HolderID,
					TokenID:         params[i].TokenID,
					ContractAddress: params[i].ContractAddress,
					Chain:           params[i].Chain,
				}
			}
		})

		return tokens, errors
	}
}

func loadTokensByCollectionID(q *db.Queries) func(context.Context, []IDAndLimit) ([][]db.Token, []error) {
	return func(ctx context.Context, collectionIDs []IDAndLimit) ([][]db.Token, []error) {
		tokens := make([][]db.Token, len(collectionIDs))
		errors := make([]error, len(collectionIDs))

		params := make([]db.GetTokensByCollectionIdBatchParams, len(collectionIDs))
		for i, collectionID := range collectionIDs {
			maybeNull := sql.NullInt32{}
			if collectionID.Limit != nil {
				maybeNull = sql.NullInt32{Int32: int32(*collectionID.Limit), Valid: true}
			}
			params[i] = db.GetTokensByCollectionIdBatchParams{
				CollectionID: collectionID.ID,
				Limit:        maybeNull,
			}
		}

		b := q.GetTokensByCollectionIdBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, t []db.Token, err error) {
			tokens[i], errors[i] = t, err
		})

		return tokens, errors
	}
}

func loadOwnerByTokenID(q *db.Queries) func(context.Context, []persist.DBID) ([]db.User, []error) {
	return func(ctx context.Context, tokenIDs []persist.DBID) ([]db.User, []error) {
		users := make([]db.User, len(tokenIDs))
		errors := make([]error, len(tokenIDs))

		b := q.GetTokenOwnerByIDBatch(ctx, tokenIDs)
		defer b.Close()

		b.QueryRow(func(i int, u db.User, err error) {
			users[i], errors[i] = u, err
		})

		return users, errors
	}
}

func loadTokensByWalletID(q *db.Queries) func(context.Context, []persist.DBID) ([][]db.Token, []error) {
	return func(ctx context.Context, walletIds []persist.DBID) ([][]db.Token, []error) {
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
		})

		return tokens, errors
	}
}

func loadTokensByUserID(q *db.Queries) func(context.Context, []db.GetTokensByUserIdBatchParams) ([][]db.Token, []error) {
	return func(ctx context.Context, params []db.GetTokensByUserIdBatchParams) ([][]db.Token, []error) {
		tokens := make([][]db.Token, len(params))
		errors := make([]error, len(params))

		b := q.GetTokensByUserIdBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, t []db.Token, err error) {
			tokens[i], errors[i] = t, err
		})

		return tokens, errors
	}
}

func loadTokensByUserIDAndChain(q *db.Queries) func(context.Context, []IDAndChain) ([][]db.Token, []error) {
	return func(ctx context.Context, userIDsAndChains []IDAndChain) ([][]db.Token, []error) {
		tokens := make([][]db.Token, len(userIDsAndChains))
		errors := make([]error, len(userIDsAndChains))

		params := make([]db.GetTokensByUserIdAndChainBatchParams, len(userIDsAndChains))
		for i, userIDAndChain := range userIDsAndChains {
			params[i] = db.GetTokensByUserIdAndChainBatchParams{
				OwnerUserID: userIDAndChain.ID,
				Chain:       userIDAndChain.Chain,
			}
		}

		b := q.GetTokensByUserIdAndChainBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, t []db.Token, err error) {
			tokens[i], errors[i] = t, err
		})

		return tokens, errors
	}
}

func loadNewTokensByFeedEventID(q *db.Queries) func(context.Context, []persist.DBID) ([][]db.Token, []error) {
	return func(ctx context.Context, tokenIDs []persist.DBID) ([][]db.Token, []error) {
		tokens := make([][]db.Token, len(tokenIDs))
		errors := make([]error, len(tokenIDs))

		b := q.GetNewTokensByFeedEventIdBatch(ctx, tokenIDs)
		defer b.Close()

		b.Query(func(i int, t []db.Token, err error) {
			tokens[i], errors[i] = t, err
		})

		return tokens, errors
	}
}

func loadContractByContractID(q *db.Queries) func(context.Context, []persist.DBID) ([]db.Contract, []error) {
	return func(ctx context.Context, contractIDs []persist.DBID) ([]db.Contract, []error) {
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

func loadContractByChainAddress(q *db.Queries) func(context.Context, []persist.ChainAddress) ([]db.Contract, []error) {
	return func(ctx context.Context, chainAddresses []persist.ChainAddress) ([]db.Contract, []error) {
		contracts := make([]db.Contract, len(chainAddresses))
		errors := make([]error, len(chainAddresses))

		asParams := make([]db.GetContractByChainAddressBatchParams, len(chainAddresses))
		for i, chainAddress := range chainAddresses {
			asParams[i] = db.GetContractByChainAddressBatchParams{
				Chain:   chainAddress.Chain(),
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

func loadContractsByCreatorID(q *db.Queries) func(context.Context, []db.GetCreatedContractsBatchPaginateParams) ([][]db.Contract, []error) {
	return func(ctx context.Context, params []db.GetCreatedContractsBatchPaginateParams) ([][]db.Contract, []error) {
		contracts := make([][]db.Contract, len(params))
		errors := make([]error, len(params))

		b := q.GetCreatedContractsBatchPaginate(ctx, params)
		defer b.Close()

		b.Query(func(i int, c []db.Contract, err error) {
			contracts[i], errors[i] = c, err
		})

		return contracts, errors
	}
}

func loadContractsByParentID(q *db.Queries) func(context.Context, []db.GetChildContractsByParentIDBatchPaginateParams) ([][]db.Contract, []error) {
	return func(ctx context.Context, params []db.GetChildContractsByParentIDBatchPaginateParams) ([][]db.Contract, []error) {
		contracts := make([][]db.Contract, len(params))
		errors := make([]error, len(params))

		b := q.GetChildContractsByParentIDBatchPaginate(ctx, params)
		defer b.Close()

		b.Query(func(i int, c []db.Contract, err error) {
			contracts[i], errors[i] = c, err
		})

		return contracts, errors
	}
}

func loadContractsDisplayedByUserID(q *db.Queries) func(context.Context, []persist.DBID) ([][]db.Contract, []error) {
	return func(ctx context.Context, contractIDs []persist.DBID) ([][]db.Contract, []error) {
		contracts := make([][]db.Contract, len(contractIDs))
		errors := make([]error, len(contractIDs))

		b := q.GetContractsDisplayedByUserIDBatch(ctx, contractIDs)
		defer b.Close()

		b.Query(func(i int, c []db.Contract, err error) {
			contracts[i], errors[i] = c, err
		})

		return contracts, errors
	}
}

func loadEventById(q *db.Queries) func(context.Context, []persist.DBID) ([]db.FeedEvent, []error) {
	return func(ctx context.Context, eventIds []persist.DBID) ([]db.FeedEvent, []error) {
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

func loadPostById(q *db.Queries) func(context.Context, []persist.DBID) ([]db.Post, []error) {
	return func(ctx context.Context, postIDs []persist.DBID) ([]db.Post, []error) {
		posts := make([]db.Post, len(postIDs))
		errors := make([]error, len(postIDs))

		b := q.GetPostByIdBatch(ctx, postIDs)
		defer b.Close()

		b.QueryRow(func(i int, p db.Post, err error) {
			posts[i] = p
			errors[i] = err

			if errors[i] == pgx.ErrNoRows {
				errors[i] = persist.ErrPostNotFoundByID{ID: postIDs[i]}
			}
		})

		return posts, errors
	}
}

func loadPostsPaginatedByContractID(q *db.Queries) func(context.Context, []db.PaginatePostsByContractIDParams) ([][]db.Post, []error) {
	return func(ctx context.Context, postIDs []db.PaginatePostsByContractIDParams) ([][]db.Post, []error) {
		events := make([][]db.Post, len(postIDs))
		errors := make([]error, len(postIDs))

		b := q.PaginatePostsByContractID(ctx, postIDs)
		defer b.Close()

		b.Query(func(i int, p []db.Post, err error) {
			events[i], errors[i] = p, err
		})

		return events, errors
	}
}
func loadUserNotifications(q *db.Queries) func(context.Context, []db.GetUserNotificationsBatchParams) ([][]db.Notification, []error) {
	return func(ctx context.Context, params []db.GetUserNotificationsBatchParams) ([][]db.Notification, []error) {
		notifs := make([][]db.Notification, len(params))
		errors := make([]error, len(params))

		b := q.GetUserNotificationsBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, ntfs []db.Notification, err error) {
			notifs[i] = ntfs
			errors[i] = err
		})

		return notifs, errors
	}
}

func loadAdmireById(q *db.Queries) func(context.Context, []persist.DBID) ([]db.Admire, []error) {
	return func(ctx context.Context, admireIDs []persist.DBID) ([]db.Admire, []error) {
		admires := make([]db.Admire, len(admireIDs))
		errors := make([]error, len(admireIDs))

		rows, err := q.GetAdmiresByAdmireIDs(ctx, admireIDs)
		if err != nil {
			fillErrors(errors, err)
			return admires, errors
		}

		admiresByID := make(map[persist.DBID]db.Admire)
		for _, row := range rows {
			admiresByID[row.ID] = row
		}

		for i, id := range admireIDs {
			if admire, ok := admiresByID[id]; ok {
				admires[i] = admire
			} else {
				errors[i] = persist.ErrAdmireNotFound{AdmireID: id}
			}
		}

		return admires, errors
	}
}

func loadAdmireCountByFeedEventID(q *db.Queries) func(context.Context, []persist.DBID) ([]int, []error) {
	return func(ctx context.Context, feedEventIDs []persist.DBID) ([]int, []error) {
		counts := make([]int, len(feedEventIDs))
		errors := make([]error, len(feedEventIDs))

		b := q.CountAdmiresByFeedEventIDBatch(ctx, feedEventIDs)
		defer b.Close()

		b.QueryRow(func(i int, count int64, err error) {
			counts[i], errors[i] = int(count), err
		})

		return counts, errors
	}
}

func loadAdmireCountByPostID(q *db.Queries) func(context.Context, []persist.DBID) ([]int, []error) {
	return func(ctx context.Context, postIDs []persist.DBID) ([]int, []error) {
		counts := make([]int, len(postIDs))
		errors := make([]error, len(postIDs))

		b := q.CountAdmiresByPostIDBatch(ctx, postIDs)
		defer b.Close()

		b.QueryRow(func(i int, count int64, err error) {
			counts[i], errors[i] = int(count), err
		})

		return counts, errors
	}
}

func loadAdmiresByFeedEventID(q *db.Queries) func(context.Context, []db.PaginateAdmiresByFeedEventIDBatchParams) ([][]db.Admire, []error) {
	return func(ctx context.Context, params []db.PaginateAdmiresByFeedEventIDBatchParams) ([][]db.Admire, []error) {
		admires := make([][]db.Admire, len(params))
		errors := make([]error, len(params))

		b := q.PaginateAdmiresByFeedEventIDBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, admrs []db.Admire, err error) {
			admires[i] = admrs
			errors[i] = err
		})

		return admires, errors
	}
}

func loadAdmiresByPostID(q *db.Queries) func(context.Context, []db.PaginateAdmiresByPostIDBatchParams) ([][]db.Admire, []error) {
	return func(ctx context.Context, params []db.PaginateAdmiresByPostIDBatchParams) ([][]db.Admire, []error) {
		admires := make([][]db.Admire, len(params))
		errors := make([]error, len(params))

		b := q.PaginateAdmiresByPostIDBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, admrs []db.Admire, err error) {
			admires[i] = admrs
			errors[i] = err
		})

		return admires, errors
	}
}

func loadCommentById(q *db.Queries) func(context.Context, []persist.DBID) ([]db.Comment, []error) {
	return func(ctx context.Context, commentIDs []persist.DBID) ([]db.Comment, []error) {
		comments := make([]db.Comment, len(commentIDs))
		errors := make([]error, len(commentIDs))

		rows, err := q.GetCommentsByCommentIDs(ctx, commentIDs)
		if err != nil {
			fillErrors(errors, err)
			return comments, errors
		}

		commentsByID := make(map[persist.DBID]db.Comment)
		for _, row := range rows {
			commentsByID[row.ID] = row
		}

		for i, id := range commentIDs {
			if comment, ok := commentsByID[id]; ok {
				comments[i] = comment
			} else {
				errors[i] = persist.ErrCommentNotFound{ID: id}
			}
		}

		return comments, errors
	}
}

func loadCommentCountByFeedEventID(q *db.Queries) func(context.Context, []persist.DBID) ([]int, []error) {
	return func(ctx context.Context, feedEventIDs []persist.DBID) ([]int, []error) {
		counts := make([]int, len(feedEventIDs))
		errors := make([]error, len(feedEventIDs))

		b := q.CountCommentsByFeedEventIDBatch(ctx, feedEventIDs)
		defer b.Close()

		b.QueryRow(func(i int, count int64, err error) {
			counts[i], errors[i] = int(count), err
		})

		return counts, errors
	}
}

func loadCommentCountByPostID(q *db.Queries) func(context.Context, []persist.DBID) ([]int, []error) {
	return func(ctx context.Context, postIDs []persist.DBID) ([]int, []error) {
		counts := make([]int, len(postIDs))
		errors := make([]error, len(postIDs))

		b := q.CountCommentsByPostIDBatch(ctx, postIDs)
		defer b.Close()

		b.QueryRow(func(i int, count int64, err error) {
			counts[i], errors[i] = int(count), err
		})

		return counts, errors
	}
}

func loadCommentsByFeedEventID(q *db.Queries) func(context.Context, []db.PaginateCommentsByFeedEventIDBatchParams) ([][]db.Comment, []error) {
	return func(ctx context.Context, params []db.PaginateCommentsByFeedEventIDBatchParams) ([][]db.Comment, []error) {
		comments := make([][]db.Comment, len(params))
		errors := make([]error, len(params))

		b := q.PaginateCommentsByFeedEventIDBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, cmts []db.Comment, err error) {
			comments[i] = cmts
			errors[i] = err
		})

		return comments, errors
	}
}

func loadCommentsByPostID(q *db.Queries) func(context.Context, []db.PaginateCommentsByPostIDBatchParams) ([][]db.Comment, []error) {
	return func(ctx context.Context, params []db.PaginateCommentsByPostIDBatchParams) ([][]db.Comment, []error) {
		comments := make([][]db.Comment, len(params))
		errors := make([]error, len(params))

		b := q.PaginateCommentsByPostIDBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, cmts []db.Comment, err error) {
			comments[i] = cmts
			errors[i] = err
		})

		return comments, errors
	}
}

func loadRepliesByCommentID(q *db.Queries) func(context.Context, []db.PaginateRepliesByCommentIDBatchParams) ([][]db.Comment, []error) {
	return func(ctx context.Context, params []db.PaginateRepliesByCommentIDBatchParams) ([][]db.Comment, []error) {
		comments := make([][]db.Comment, len(params))
		errors := make([]error, len(params))

		b := q.PaginateRepliesByCommentIDBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, cmts []db.Comment, err error) {
			comments[i] = cmts
			errors[i] = err
		})

		return comments, errors
	}
}

func loadReplyCountByCommentID(q *db.Queries) func(context.Context, []persist.DBID) ([]int, []error) {
	return func(ctx context.Context, commentIDs []persist.DBID) ([]int, []error) {
		counts := make([]int, len(commentIDs))
		errors := make([]error, len(commentIDs))

		b := q.CountRepliesByCommentIDBatch(ctx, commentIDs)
		defer b.Close()

		b.QueryRow(func(i int, count int64, err error) {
			counts[i], errors[i] = int(count), err
		})

		return counts, errors
	}
}

func loadInteractionCountByFeedEventID(q *db.Queries) func(context.Context, []db.CountInteractionsByFeedEventIDBatchParams) ([][]db.CountInteractionsByFeedEventIDBatchRow, []error) {
	return func(ctx context.Context, params []db.CountInteractionsByFeedEventIDBatchParams) ([][]db.CountInteractionsByFeedEventIDBatchRow, []error) {
		rows := make([][]db.CountInteractionsByFeedEventIDBatchRow, len(params))
		errors := make([]error, len(params))

		b := q.CountInteractionsByFeedEventIDBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, r []db.CountInteractionsByFeedEventIDBatchRow, err error) {
			rows[i], errors[i] = r, err
		})

		return rows, errors
	}
}

func loadInteractionsByFeedEventID(q *db.Queries) func(context.Context, []db.PaginateInteractionsByFeedEventIDBatchParams) ([][]db.PaginateInteractionsByFeedEventIDBatchRow, []error) {
	return func(ctx context.Context, params []db.PaginateInteractionsByFeedEventIDBatchParams) ([][]db.PaginateInteractionsByFeedEventIDBatchRow, []error) {
		interactions := make([][]db.PaginateInteractionsByFeedEventIDBatchRow, len(params))
		errors := make([]error, len(params))

		b := q.PaginateInteractionsByFeedEventIDBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, r []db.PaginateInteractionsByFeedEventIDBatchRow, err error) {
			interactions[i], errors[i] = r, err
		})

		return interactions, errors
	}
}

func loadInteractionCountByPostID(q *db.Queries) func(context.Context, []db.CountInteractionsByPostIDBatchParams) ([][]db.CountInteractionsByPostIDBatchRow, []error) {
	return func(ctx context.Context, params []db.CountInteractionsByPostIDBatchParams) ([][]db.CountInteractionsByPostIDBatchRow, []error) {
		rows := make([][]db.CountInteractionsByPostIDBatchRow, len(params))
		errors := make([]error, len(params))

		b := q.CountInteractionsByPostIDBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, r []db.CountInteractionsByPostIDBatchRow, err error) {
			rows[i], errors[i] = r, err
		})

		return rows, errors
	}
}

func loadInteractionsByPostID(q *db.Queries) func(context.Context, []db.PaginateInteractionsByPostIDBatchParams) ([][]db.PaginateInteractionsByPostIDBatchRow, []error) {
	return func(ctx context.Context, params []db.PaginateInteractionsByPostIDBatchParams) ([][]db.PaginateInteractionsByPostIDBatchRow, []error) {
		interactions := make([][]db.PaginateInteractionsByPostIDBatchRow, len(params))
		errors := make([]error, len(params))

		b := q.PaginateInteractionsByPostIDBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, r []db.PaginateInteractionsByPostIDBatchRow, err error) {
			interactions[i], errors[i] = r, err
		})

		return interactions, errors
	}
}

func loadAdmireByActorIDAndFeedEventID(q *db.Queries) func(context.Context, []db.GetAdmireByActorIDAndFeedEventIDParams) ([]db.Admire, []error) {
	return func(ctx context.Context, params []db.GetAdmireByActorIDAndFeedEventIDParams) ([]db.Admire, []error) {
		results := make([]db.Admire, len(params))
		errors := make([]error, len(params))

		b := q.GetAdmireByActorIDAndFeedEventID(ctx, params)
		defer b.Close()

		b.QueryRow(func(i int, admire db.Admire, err error) {
			if err == pgx.ErrNoRows {
				err = persist.ErrAdmireNotFound{ActorID: params[i].ActorID, FeedEventID: params[i].FeedEventID}
			}
			results[i], errors[i] = admire, err
		})

		return results, errors
	}
}

func loadAdmireByActorIDAndPostID(q *db.Queries) func(context.Context, []db.GetAdmireByActorIDAndPostIDParams) ([]db.Admire, []error) {
	return func(ctx context.Context, params []db.GetAdmireByActorIDAndPostIDParams) ([]db.Admire, []error) {
		results := make([]db.Admire, len(params))
		errors := make([]error, len(params))

		b := q.GetAdmireByActorIDAndPostID(ctx, params)
		defer b.Close()

		b.QueryRow(func(i int, admire db.Admire, err error) {
			if err == pgx.ErrNoRows {
				err = persist.ErrAdmireNotFound{ActorID: params[i].ActorID, PostID: params[i].PostID}
			}
			results[i], errors[i] = admire, err
		})

		return results, errors
	}
}

func loadMediaByTokenID(q *db.Queries) func(context.Context, []persist.DBID) ([]db.TokenMedia, []error) {
	return func(ctx context.Context, tokenIDs []persist.DBID) ([]db.TokenMedia, []error) {
		results := make([]db.TokenMedia, len(tokenIDs))
		errors := make([]error, len(tokenIDs))

		b := q.GetMediaByTokenID(ctx, tokenIDs)
		defer b.Close()

		b.QueryRow(func(i int, media db.TokenMedia, err error) {
			if err == pgx.ErrNoRows {
				err = persist.ErrMediaNotFound{TokenID: tokenIDs[i]}
			}
			results[i], errors[i] = media, err
		})

		return results, errors
	}
}

func loadContractCreatorByContractID(q *db.Queries) func(ctx context.Context, keys []persist.DBID) ([]db.ContractCreator, []error) {
	return func(ctx context.Context, contractIDs []persist.DBID) ([]db.ContractCreator, []error) {
		rows, err := q.GetContractCreatorsByIds(ctx, util.StringersToStrings(contractIDs))
		if err != nil {
			return emptyResultsWithError[db.ContractCreator](len(contractIDs), err)
		}

		keyFunc := func(row db.ContractCreator) persist.DBID { return row.ContractID }
		onNotFound := func(contractID persist.DBID) (db.ContractCreator, error) {
			return db.ContractCreator{}, persist.ErrContractCreatorNotFound{ContractID: contractID}
		}

		return fillUnnestedJoinResults(contractIDs, rows, keyFunc, onNotFound)
	}
}

func loadProfileImageByID(q *db.Queries) func(context.Context, []db.GetProfileImageByIDParams) ([]db.ProfileImage, []error) {
	return func(ctx context.Context, params []db.GetProfileImageByIDParams) ([]db.ProfileImage, []error) {
		results := make([]db.ProfileImage, len(params))
		errors := make([]error, len(params))

		b := q.GetProfileImageByID(ctx, params)
		defer b.Close()

		b.QueryRow(func(i int, media db.ProfileImage, err error) {
			if err == pgx.ErrNoRows {
				err = persist.ErrProfileImageNotFound{Err: err, ProfileImageID: params[i].ID}
			}
			results[i], errors[i] = media, err
		})

		return results, errors
	}
}

func loadGalleryTokenPreviewsByID(q *db.Queries) func(context.Context, []persist.DBID) ([][]db.TokenMedia, []error) {
	return func(ctx context.Context, keys []persist.DBID) ([][]db.TokenMedia, []error) {
		results := make([][]db.TokenMedia, len(keys))
		errors := make([]error, len(keys))

		b := q.GetGalleryTokenMediasByGalleryIDBatch(ctx, keys)
		defer b.Close()

		b.Query(func(i int, medias []db.TokenMedia, err error) {
			if err == pgx.ErrNoRows {
				err = persist.ErrGalleryNotFound{ID: keys[i]}
			}
			results[i], errors[i] = medias, err
		})

		return results, errors
	}
}
