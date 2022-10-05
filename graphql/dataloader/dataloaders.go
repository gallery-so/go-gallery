//go:generate go run github.com/gallery-so/dataloaden UserLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.User
//go:generate go run github.com/gallery-so/dataloaden UsersLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.User
//go:generate go run github.com/gallery-so/dataloaden UserLoaderByAddress github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.User
//go:generate go run github.com/gallery-so/dataloaden UserLoaderByString string github.com/mikeydub/go-gallery/db/gen/coredb.User
//go:generate go run github.com/gallery-so/dataloaden UsersLoaderByString string []github.com/mikeydub/go-gallery/db/gen/coredb.User
//go:generate go run github.com/gallery-so/dataloaden GalleryLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Gallery
//go:generate go run github.com/gallery-so/dataloaden GalleriesLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Gallery
//go:generate go run github.com/gallery-so/dataloaden CollectionLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Collection
//go:generate go run github.com/gallery-so/dataloaden CollectionsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Collection
//go:generate go run github.com/gallery-so/dataloaden MembershipLoaderById github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Membership
//go:generate go run github.com/gallery-so/dataloaden WalletLoaderById github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Wallet
//go:generate go run github.com/gallery-so/dataloaden WalletLoaderByChainAddress github.com/mikeydub/go-gallery/service/persist.ChainAddress github.com/mikeydub/go-gallery/db/gen/coredb.Wallet
//go:generate go run github.com/gallery-so/dataloaden WalletsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Wallet
//go:generate go run github.com/gallery-so/dataloaden TokenLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Token
//go:generate go run github.com/gallery-so/dataloaden TokensLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Token
//go:generate go run github.com/gallery-so/dataloaden TokensLoaderByIDAndChain github.com/mikeydub/go-gallery/graphql/dataloader.IDAndChain []github.com/mikeydub/go-gallery/db/gen/coredb.Token
//go:generate go run github.com/gallery-so/dataloaden ContractLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Contract
//go:generate go run github.com/gallery-so/dataloaden ContractsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Contract
//go:generate go run github.com/gallery-so/dataloaden ContractLoaderByChainAddress github.com/mikeydub/go-gallery/service/persist.ChainAddress github.com/mikeydub/go-gallery/db/gen/coredb.Contract
//go:generate go run github.com/gallery-so/dataloaden GlobalFeedLoader github.com/mikeydub/go-gallery/db/gen/coredb.GetGlobalFeedViewBatchParams []github.com/mikeydub/go-gallery/db/gen/coredb.FeedEvent
//go:generate go run github.com/gallery-so/dataloaden UserFeedLoader github.com/mikeydub/go-gallery/db/gen/coredb.GetUserFeedViewBatchParams []github.com/mikeydub/go-gallery/db/gen/coredb.FeedEvent
//go:generate go run github.com/gallery-so/dataloaden EventLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.FeedEvent
//go:generate go run github.com/gallery-so/dataloaden AdmireLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Admire
//go:generate go run github.com/gallery-so/dataloaden AdmiresLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Admire
//go:generate go run github.com/gallery-so/dataloaden CommentLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID github.com/mikeydub/go-gallery/db/gen/coredb.Comment
//go:generate go run github.com/gallery-so/dataloaden CommentsLoaderByID github.com/mikeydub/go-gallery/service/persist.DBID []github.com/mikeydub/go-gallery/db/gen/coredb.Comment

package dataloader

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/jackc/pgx/v4"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

type IDAndChain struct {
	ID    persist.DBID
	Chain persist.Chain
}

// Loaders will cache and batch lookups. They are short-lived and should never persist beyond
// a single request, nor should they be shared between requests (since the data returned is
// relative to the current request context, including the user and their auth status).
type Loaders struct {

	// Every entry here must have a corresponding entry in the Clear___Caches methods below

	UserByUserId             *UserLoaderByID
	UserByUsername           *UserLoaderByString
	UsersWithTrait           *UsersLoaderByString
	GalleryByGalleryId       *GalleryLoaderByID
	GalleryByCollectionId    *GalleryLoaderByID
	GalleriesByUserId        *GalleriesLoaderByID
	CollectionByCollectionId *CollectionLoaderByID
	CollectionsByGalleryId   *CollectionsLoaderByID
	MembershipByMembershipId *MembershipLoaderById
	WalletByWalletId         *WalletLoaderById
	WalletsByUserID          *WalletsLoaderByID
	WalletByChainAddress     *WalletLoaderByChainAddress
	TokenByTokenID           *TokenLoaderByID
	TokensByCollectionID     *TokensLoaderByID
	TokensByWalletID         *TokensLoaderByID
	TokensByUserID           *TokensLoaderByID
	TokensByUserIDAndChain   *TokensLoaderByIDAndChain
	NewTokensByFeedEventID   *TokensLoaderByID
	ContractByContractId     *ContractLoaderByID
	ContractsByUserID        *ContractsLoaderByID
	ContractByChainAddress   *ContractLoaderByChainAddress
	FollowersByUserId        *UsersLoaderByID
	FollowingByUserId        *UsersLoaderByID
	GlobalFeed               *GlobalFeedLoader
	FeedByUserId             *UserFeedLoader
	EventByEventId           *EventLoaderByID
	AdmireByAdmireId         *AdmireLoaderByID
	AdmiresByFeedEventId     *AdmiresLoaderByID
	CommentByCommentId       *CommentLoaderByID
	CommentsByFeedEventId    *CommentsLoaderByID
}

func NewLoaders(ctx context.Context, q *db.Queries, disableCaching bool) *Loaders {
	subscriptionRegistry := make([]interface{}, 0)
	mutexRegistry := make([]*sync.Mutex, 0)

	defaults := settings{
		ctx:                  ctx,
		maxBatchOne:          100,
		maxBatchMany:         10,
		waitTime:             2 * time.Millisecond,
		disableCaching:       disableCaching,
		publishResults:       true,
		subscriptionRegistry: &subscriptionRegistry,
		mutexRegistry:        &mutexRegistry,
	}

	loaders := &Loaders{}

	loaders.UserByUserId = NewUserLoaderByID(defaults, loadUserByUserId(q), UserLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(user db.User) persist.DBID { return user.ID },
	})

	loaders.UserByUsername = NewUserLoaderByString(defaults, loadUserByUsername(q), UserLoaderByStringCacheSubscriptions{
		AutoCacheWithKey: func(user db.User) string { return user.Username.String },
	})

	loaders.UsersWithTrait = NewUsersLoaderByString(defaults, loadUsersWithTrait(q))

	loaders.GalleryByGalleryId = NewGalleryLoaderByID(defaults, loadGalleryByGalleryId(q), GalleryLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(gallery db.Gallery) persist.DBID { return gallery.ID },
	})

	loaders.GalleryByCollectionId = NewGalleryLoaderByID(defaults, loadGalleryByCollectionId(q), GalleryLoaderByIDCacheSubscriptions{
		AutoCacheWithKeys: func(gallery db.Gallery) []persist.DBID { return gallery.Collections },
	})

	loaders.GalleriesByUserId = NewGalleriesLoaderByID(defaults, loadGalleriesByUserId(q))

	loaders.CollectionByCollectionId = NewCollectionLoaderByID(defaults, loadCollectionByCollectionId(q), CollectionLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(collection db.Collection) persist.DBID { return collection.ID },
	})

	loaders.CollectionsByGalleryId = NewCollectionsLoaderByID(defaults, loadCollectionsByGalleryId(q))

	loaders.MembershipByMembershipId = NewMembershipLoaderById(defaults, loadMembershipByMembershipId(q), MembershipLoaderByIdCacheSubscriptions{
		AutoCacheWithKey: func(membership db.Membership) persist.DBID { return membership.ID },
	})

	loaders.WalletByWalletId = NewWalletLoaderById(defaults, loadWalletByWalletId(q), WalletLoaderByIdCacheSubscriptions{
		AutoCacheWithKey: func(wallet db.Wallet) persist.DBID { return wallet.ID },
	})

	loaders.WalletsByUserID = NewWalletsLoaderByID(defaults, loadWalletsByUserId(q))

	loaders.WalletByChainAddress = NewWalletLoaderByChainAddress(defaults, loadWalletByChainAddress(q), WalletLoaderByChainAddressCacheSubscriptions{
		AutoCacheWithKey: func(wallet db.Wallet) persist.ChainAddress {
			return persist.NewChainAddress(wallet.Address, persist.Chain(wallet.Chain.Int32))
		},
	})

	loaders.FollowersByUserId = NewUsersLoaderByID(defaults, loadFollowersByUserId(q))

	loaders.FollowingByUserId = NewUsersLoaderByID(defaults, loadFollowingByUserId(q))

	loaders.TokenByTokenID = NewTokenLoaderByID(defaults, loadTokenByTokenID(q), TokenLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(token db.Token) persist.DBID { return token.ID },
	})

	loaders.TokensByCollectionID = NewTokensLoaderByID(defaults, loadTokensByCollectionID(q))

	loaders.TokensByWalletID = NewTokensLoaderByID(defaults, loadTokensByWalletID(q))

	loaders.TokensByUserID = NewTokensLoaderByID(defaults, loadTokensByUserID(q))

	loaders.TokensByUserIDAndChain = NewTokensLoaderByIDAndChain(defaults, loadTokensByUserIDAndChain(q))

	loaders.NewTokensByFeedEventID = NewTokensLoaderByID(defaults, loadNewTokensByFeedEventID(q))

	loaders.ContractByContractId = NewContractLoaderByID(defaults, loadContractByContractID(q), ContractLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(contract db.Contract) persist.DBID { return contract.ID },
	})

	loaders.ContractByChainAddress = NewContractLoaderByChainAddress(defaults, loadContractByChainAddress(q), ContractLoaderByChainAddressCacheSubscriptions{
		AutoCacheWithKey: func(contract db.Contract) persist.ChainAddress {
			return persist.NewChainAddress(contract.Address, persist.Chain(contract.Chain.Int32))
		},
	})

	loaders.ContractsByUserID = NewContractsLoaderByID(defaults, loadContractsByUserID(q))

	loaders.EventByEventId = NewEventLoaderByID(defaults, loadEventById(q), EventLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(event db.FeedEvent) persist.DBID { return event.ID },
	})

	loaders.FeedByUserId = NewUserFeedLoader(defaults, loadUserFeed(q))

	loaders.GlobalFeed = NewGlobalFeedLoader(defaults, loadGlobalFeed(q))

	loaders.AdmireByAdmireId = NewAdmireLoaderByID(defaults, loadAdmireById(q), AdmireLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(admire db.Admire) persist.DBID { return admire.ID },
	})

	loaders.AdmiresByFeedEventId = NewAdmiresLoaderByID(defaults, loadAdmiresByFeedEventId(q))

	loaders.CommentByCommentId = NewCommentLoaderByID(defaults, loadCommentById(q), CommentLoaderByIDCacheSubscriptions{
		AutoCacheWithKey: func(comment db.Comment) persist.DBID { return comment.ID },
	})

	loaders.CommentsByFeedEventId = NewCommentsLoaderByID(defaults, loadCommentsByFeedEventId(q))

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
				errors[i] = persist.ErrGalleryNotFoundByID{ID: galleryIds[i]}
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
				errors[i] = persist.ErrGalleryNotFoundByCollectionID{ID: collectionIds[i]}
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
				Chain:   sql.NullInt32{Int32: int32(chainAddress.Chain()), Valid: true},
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

func loadTokensByCollectionID(q *db.Queries) func(context.Context, []persist.DBID) ([][]db.Token, []error) {
	return func(ctx context.Context, collectionIDs []persist.DBID) ([][]db.Token, []error) {
		tokens := make([][]db.Token, len(collectionIDs))
		errors := make([]error, len(collectionIDs))

		b := q.GetTokensByCollectionIdBatch(ctx, collectionIDs)
		defer b.Close()

		b.Query(func(i int, t []db.Token, err error) {
			tokens[i], errors[i] = t, err
		})

		return tokens, errors
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

func loadTokensByUserID(q *db.Queries) func(context.Context, []persist.DBID) ([][]db.Token, []error) {
	return func(ctx context.Context, userIDs []persist.DBID) ([][]db.Token, []error) {
		tokens := make([][]db.Token, len(userIDs))
		errors := make([]error, len(userIDs))

		b := q.GetTokensByUserIdBatch(ctx, userIDs)
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
				Chain:       sql.NullInt32{Int32: int32(userIDAndChain.Chain), Valid: true},
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

func loadContractsByUserID(q *db.Queries) func(context.Context, []persist.DBID) ([][]db.Contract, []error) {
	return func(ctx context.Context, contractIDs []persist.DBID) ([][]db.Contract, []error) {
		contracts := make([][]db.Contract, len(contractIDs))
		errors := make([]error, len(contractIDs))

		b := q.GetContractsByUserIDBatch(ctx, contractIDs)
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

func loadUserFeed(q *db.Queries) func(context.Context, []db.GetUserFeedViewBatchParams) ([][]db.FeedEvent, []error) {
	return func(ctx context.Context, params []db.GetUserFeedViewBatchParams) ([][]db.FeedEvent, []error) {
		events := make([][]db.FeedEvent, len(params))
		errors := make([]error, len(params))

		b := q.GetUserFeedViewBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, evts []db.FeedEvent, err error) {
			events[i] = evts
			errors[i] = err
		})

		return events, errors
	}
}

func loadGlobalFeed(q *db.Queries) func(context.Context, []db.GetGlobalFeedViewBatchParams) ([][]db.FeedEvent, []error) {
	return func(ctx context.Context, params []db.GetGlobalFeedViewBatchParams) ([][]db.FeedEvent, []error) {
		events := make([][]db.FeedEvent, len(params))
		errors := make([]error, len(params))

		b := q.GetGlobalFeedViewBatch(ctx, params)
		defer b.Close()

		b.Query(func(i int, evts []db.FeedEvent, err error) {
			events[i] = evts
			errors[i] = err
		})

		return events, errors
	}
}

func loadAdmireById(q *db.Queries) func(context.Context, []persist.DBID) ([]db.Admire, []error) {
	return func(ctx context.Context, admireIds []persist.DBID) ([]db.Admire, []error) {
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

func loadAdmiresByFeedEventId(q *db.Queries) func(context.Context, []persist.DBID) ([][]db.Admire, []error) {
	return func(ctx context.Context, ids []persist.DBID) ([][]db.Admire, []error) {
		admires := make([][]db.Admire, len(ids))
		errors := make([]error, len(ids))

		b := q.GetAdmiresByFeedEventIDBatch(ctx, ids)
		defer b.Close()

		b.Query(func(i int, admrs []db.Admire, err error) {
			admires[i] = admrs
			errors[i] = err
		})

		return admires, errors
	}
}

func loadCommentById(q *db.Queries) func(context.Context, []persist.DBID) ([]db.Comment, []error) {
	return func(ctx context.Context, commentIds []persist.DBID) ([]db.Comment, []error) {
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

func loadCommentsByFeedEventId(q *db.Queries) func(context.Context, []persist.DBID) ([][]db.Comment, []error) {
	return func(ctx context.Context, ids []persist.DBID) ([][]db.Comment, []error) {
		comments := make([][]db.Comment, len(ids))
		errors := make([]error, len(ids))

		b := q.GetCommentsByFeedEventIDBatch(ctx, ids)
		defer b.Close()

		b.Query(func(i int, cmts []db.Comment, err error) {
			comments[i] = cmts
			errors[i] = err
		})

		return comments, errors
	}
}
