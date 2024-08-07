package publicapi

import (
	"context"
	"github.com/mikeydub/go-gallery/service/auth/privy"
	"net/http"
	"time"

	"github.com/mikeydub/go-gallery/service/farcaster"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/tracing"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	shell "github.com/ipfs/go-ipfs-api"
	magicclient "github.com/magiclabs/magic-admin-go/client"

	admin "github.com/mikeydub/go-gallery/adminapi"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/graphql/apq"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/limiters"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/highlight"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/service/tokenmanage"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

const apiContextKey = "publicapi.api"

type PublicAPI struct {
	repos         *postgres.Repositories
	queries       *db.Queries
	loaders       *dataloader.Loaders
	validator     *validator.Validate
	APQ           *apq.APQCache
	Auth          *AuthAPI
	Collection    *CollectionAPI
	Gallery       *GalleryAPI
	User          *UserAPI
	Token         *TokenAPI
	Contract      *ContractAPI
	Community     *CommunityAPI
	Wallet        *WalletAPI
	Misc          *MiscAPI
	Feed          *FeedAPI
	Notifications *NotificationsAPI
	Interaction   *InteractionAPI
	Admin         *admin.AdminAPI
	Merch         *MerchAPI
	Social        *SocialAPI
	Card          *CardAPI
	Search        *SearchAPI
	Mint          *MintAPI
}

func New(ctx context.Context, disableDataloaderCaching bool, repos *postgres.Repositories, queries *db.Queries, httpClient *http.Client, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, taskClient *task.Client, throttler *throttle.Locker, secrets *secretmanager.Client, apq *apq.APQCache, feedCache, socialCache, authRefreshCache, tokenManageCache, oneTimeLoginCache *redis.Cache, magicClient *magicclient.API, neynar *farcaster.NeynarAPI, mintLimiter *limiters.KeyRateLimiter) *PublicAPI {
	multichainProvider := multichain.NewMultichainProvider(ctx, repos, queries, ethClient, taskClient, tokenManageCache)
	return NewWithMultichainProvider(ctx, disableDataloaderCaching, repos, queries, httpClient, ethClient, ipfsClient, arweaveClient, storageClient, taskClient, throttler, secrets, apq, feedCache, socialCache, authRefreshCache, tokenManageCache, oneTimeLoginCache, magicClient, neynar, mintLimiter, multichainProvider)
}

func NewWithMultichainProvider(ctx context.Context, disableDataloaderCaching bool, repos *postgres.Repositories, queries *db.Queries, httpClient *http.Client, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, taskClient *task.Client, throttler *throttle.Locker, secrets *secretmanager.Client, apq *apq.APQCache, feedCache, socialCache, authRefreshCache, tokenManageCache, oneTimeLoginCache *redis.Cache, magicClient *magicclient.API, neynar *farcaster.NeynarAPI, mintLimiter *limiters.KeyRateLimiter, multichainProvider *multichain.Provider) *PublicAPI {
	loaders := dataloader.NewLoaders(ctx, queries, disableDataloaderCaching, tracing.DataloaderPreFetchHook, tracing.DataloaderPostFetchHook)
	validator := validate.WithCustomValidators()
	tokenManager := tokenmanage.New(ctx, taskClient, tokenManageCache, nil)
	privyClient := privy.NewPrivyClient(httpClient)
	highlightProvider := highlight.NewProvider(httpClient)
	return &PublicAPI{
		repos:         repos,
		queries:       queries,
		loaders:       loaders,
		validator:     validator,
		APQ:           apq,
		Auth:          &AuthAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient, multiChainProvider: multichainProvider, magicLinkClient: magicClient, oneTimeLoginCache: oneTimeLoginCache, authRefreshCache: authRefreshCache, privyClient: privyClient, neynarClient: neynar},
		Collection:    &CollectionAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient},
		Gallery:       &GalleryAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient},
		User:          &UserAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient, multichainProvider: multichainProvider, taskClient: taskClient},
		Contract:      &ContractAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient, multichainProvider: multichainProvider, taskClient: taskClient},
		Community:     &CommunityAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient, multichainProvider: multichainProvider, taskClient: taskClient},
		Token:         &TokenAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient, multichainProvider: multichainProvider, throttler: throttler, manager: tokenManager},
		Wallet:        &WalletAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient, multichainProvider: multichainProvider},
		Misc:          &MiscAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient, storageClient: storageClient},
		Feed:          &FeedAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient, cache: feedCache, taskClient: taskClient, multichainProvider: multichainProvider},
		Interaction:   &InteractionAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient},
		Notifications: &NotificationsAPI{queries: queries, loaders: loaders, validator: validator},
		Admin:         admin.NewAPI(repos, queries, authRefreshCache, validator, multichainProvider),
		Merch:         &MerchAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, ethClient: ethClient, multichainProvider: multichainProvider, secrets: secrets},
		Social:        &SocialAPI{repos: repos, queries: queries, loaders: loaders, validator: validator, redis: socialCache, httpClient: httpClient, taskClient: taskClient, neynarAPI: neynar},
		Card:          &CardAPI{validator: validator, ethClient: ethClient, multichainProvider: multichainProvider, secrets: secrets},
		Search:        &SearchAPI{queries: queries, loaders: loaders, validator: validator},
		Mint:          &MintAPI{validator: validator, highlightProvider: highlightProvider, queries: queries, taskClient: taskClient, throttler: throttler, ipRateLimiter: mintLimiter},
	}
}

// AddTo adds the specified PublicAPI to a gin context
func AddTo(ctx *gin.Context, api *PublicAPI) {
	ctx.Set(apiContextKey, api)
}

// PushTo pushes the specified PublicAPI onto the context stack and returns the new context
func PushTo(ctx context.Context, api *PublicAPI) context.Context {
	return context.WithValue(ctx, apiContextKey, api)
}

func For(ctx context.Context) *PublicAPI {
	// See if a newer PublicAPI instance has been pushed to the context stack
	if api, ok := ctx.Value(apiContextKey).(*PublicAPI); ok {
		return api
	}

	// If not, fall back to the one added to the gin context
	gc := util.MustGetGinContext(ctx)
	return gc.Value(apiContextKey).(*PublicAPI)
}

func getAuthenticatedUserID(ctx context.Context) (persist.DBID, error) {
	gc := util.MustGetGinContext(ctx)
	authError := auth.GetAuthErrorFromCtx(gc)

	if authError != nil {
		return "", authError
	}

	userID := auth.GetUserIDFromCtx(gc)
	return userID, nil
}

func getUserRoles(ctx context.Context) []persist.Role {
	gc := util.MustGetGinContext(ctx)
	return auth.GetRolesFromCtx(gc)
}

func publishEventGroup(ctx context.Context, groupID string, action persist.Action, caption *string) (*db.FeedEvent, error) {
	return event.DispatchGroup(sentryutil.NewSentryHubGinContext(ctx), groupID, action, caption)
}

// dbidCache is a lazy cache that stores DBIDs from expensive queries
type dbidCache struct {
	*redis.LazyCache
}

func newDBIDCache(cfg redis.CacheConfig, key string, ttl time.Duration, f func(context.Context) ([]persist.DBID, error)) dbidCache {
	lc := &redis.LazyCache{Cache: redis.NewCache(cfg), Key: key, TTL: ttl}
	lc.CalcFunc = func(ctx context.Context) ([]byte, error) {
		ids, err := f(ctx)
		if err != nil {
			return nil, err
		}
		cur := cursors.NewPositionCursor()
		cur.CurrentPosition = 0
		cur.IDs = ids
		b, err := cur.Pack()
		return []byte(b), err
	}
	return dbidCache{lc}
}

func (d dbidCache) Load(ctx context.Context) ([]persist.DBID, error) {
	b, err := d.LazyCache.Load(ctx)
	if err != nil {
		return nil, err
	}
	cur := cursors.NewPositionCursor()
	err = cur.Unpack(string(b))
	return cur.IDs, err
}
