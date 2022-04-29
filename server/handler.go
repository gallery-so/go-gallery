package server

import (
	"context"
	"fmt"

	"cloud.google.com/go/storage"
	gqlgen "github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/graphql/generated"
	graphql "github.com/mikeydub/go-gallery/graphql/resolver"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/event"
	"github.com/mikeydub/go-gallery/service/membership"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

func handlersInit(router *gin.Engine, repos *persist.Repositories, queries *sqlc.Queries, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, mcProvider *multichain.Provider) *gin.Engine {

	apiGroupV1 := router.Group("/glry/v1")
	apiGroupV2 := router.Group("/glry/v2")
	graphqlGroup := router.Group("/glry/graphql")

	nftHandlersInit(apiGroupV1, repos, ethClient, stg, ipfsClient, arweaveClient, stg, mcProvider)
	tokenHandlersInit(apiGroupV2, repos, ethClient, ipfsClient, arweaveClient, stg, mcProvider)
	graphqlHandlersInit(graphqlGroup, repos, queries, ethClient, ipfsClient, arweaveClient, stg, mcProvider)

	return router
}

func graphqlHandlersInit(parent *gin.RouterGroup, repos *persist.Repositories, queries *sqlc.Queries, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, mcProvider *multichain.Provider) {
	parent.POST("/query", middleware.AddAuthToContext(), graphqlHandler(repos, queries, ethClient, ipfsClient, arweaveClient, storageClient, mcProvider))

	if viper.GetString("ENV") != "production" {
		// TODO: Consider completely disabling introspection in production
		parent.GET("/playground", graphqlPlaygroundHandler())
	}
}

func graphqlHandler(repos *persist.Repositories, queries *sqlc.Queries, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, mp *multichain.Provider) gin.HandlerFunc {
	// TODO: Resolver probably doesn't need repos or ethClient once the publicAPI is done
	config := generated.Config{Resolvers: &graphql.Resolver{Repos: repos, EthClient: ethClient}}
	config.Directives.AuthRequired = graphql.AuthRequiredDirectiveHandler(ethClient)

	schema := generated.NewExecutableSchema(config)
	h := handler.NewDefaultServer(schema)
	h.AroundOperations(graphql.ScrubbedRequestLogger(schema.Schema()))
	h.AroundFields(graphql.RemapErrors)
	h.AroundResponses(graphql.AddErrorsToGin)

	h.SetRecoverFunc(func(ctx context.Context, err interface{}) error {
		gc := util.GinContextFromContext(ctx)
		if hub := sentrygin.GetHubFromContext(gc); hub != nil {
			hub.Recover(err)
		}

		return gqlgen.DefaultRecover(ctx, err)
	})

	return func(c *gin.Context) {
		c.Set(graphql.GraphQLErrorsKey, &graphql.GraphQLErrorContext{})

		hub := sentrygin.GetHubFromContext(c)
		if hub != nil {
			sentry.SetSentryAuthContext(c, hub)
		}

		defer func() {
			if hub != nil {
				for _, err := range c.Errors {
					hub.Scope().SetContext(sentry.ErrorSentryContextName, sentry.SentryErrorContext{})
					hub.CaptureException(err)
				}

				if gqlErrCtx := graphql.GqlErrorContextFromContext(c); gqlErrCtx != nil {
					for _, mappedErr := range gqlErrCtx.Errors() {
						errCtx := sentry.SentryErrorContext{}

						if mappedErr.Model != nil {
							errCtx.Mapped = true
							errCtx.MappedTo = fmt.Sprintf("%T", mappedErr.Model)
						}

						hub.Scope().SetContext(sentry.ErrorSentryContextName, errCtx)
						hub.CaptureException(mappedErr.Error)
					}
				}
			}
		}()

		event.AddTo(c, repos)
		publicapi.AddTo(c, repos, queries, ethClient, ipfsClient, arweaveClient, storageClient, mp)
		h.ServeHTTP(c.Writer, c.Request)
	}
}

// GraphQL playground GUI for experimenting and debugging
func graphqlPlaygroundHandler() gin.HandlerFunc {
	h := playground.Handler("GraphQL", "/glry/graphql/query")

	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func authHandlersInitToken(parent *gin.RouterGroup, repos *persist.Repositories, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, mcProvider *multichain.Provider) {

	usersGroup := parent.Group("/users")

	authGroup := parent.Group("/auth")

	// AUTH
	authGroup.GET("/get_preflight", middleware.AuthOptional(), getAuthPreflight(repos.UserRepository, repos.NonceRepository, repos.WalletRepository, ethClient))
	authGroup.GET("/jwt_valid", middleware.AuthOptional(), auth.ValidateJWT())
	authGroup.GET("/is_member", middleware.AuthOptional(), hasNFTs(repos.UserRepository, ethClient, membership.PremiumCards, membership.MembershipTierIDs))
	authGroup.POST("/logout", logout())

	// USER

	usersGroup.POST("/login", login(repos.UserRepository, repos.NonceRepository, repos.LoginRepository, ethClient))
	usersGroup.POST("/update/info", middleware.AuthRequired(repos.UserRepository, ethClient), updateUserInfo(repos.UserRepository, ethClient))
	usersGroup.POST("/update/addresses/add", middleware.AuthRequired(repos.UserRepository, ethClient), addUserAddressToken(repos.UserRepository, repos.NonceRepository, repos.TokenRepository, repos.ContractRepository, ethClient, ipfsClient, arweaveClient, storageClient))
	usersGroup.POST("/update/addresses/remove", middleware.AuthRequired(repos.UserRepository, ethClient), removeAddressesToken(repos.UserRepository, repos.WalletRepository))
	usersGroup.GET("/get", middleware.AuthOptional(), getUser(repos.UserRepository))
	usersGroup.GET("/get/current", middleware.AuthOptional(), getCurrentUser(repos.UserRepository))
	usersGroup.GET("/membership", getMembershipTiersToken(repos.MembershipRepository, repos.UserRepository, repos.TokenRepository, repos.GalleryTokenRepository, ethClient))
	usersGroup.POST("/create", createUserToken(repos.UserRepository, repos.NonceRepository, repos.GalleryTokenRepository, repos.TokenRepository, repos.ContractRepository, repos.WalletRepository, ethClient, ipfsClient, arweaveClient, storageClient, mcProvider))
	usersGroup.GET("/previews", getNFTPreviewsToken(repos.GalleryTokenRepository, repos.UserRepository))
	usersGroup.POST("/merge", middleware.AuthRequired(repos.UserRepository, ethClient), mergeUsers(repos.UserRepository, repos.NonceRepository, repos.WalletRepository, mcProvider))
}

func authHandlersInitNFT(parent *gin.RouterGroup, repos *persist.Repositories, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, mcProvider *multichain.Provider) {

	usersGroup := parent.Group("/users")

	authGroup := parent.Group("/auth")

	// AUTH
	authGroup.GET("/get_preflight", middleware.AuthOptional(), getAuthPreflight(repos.UserRepository, repos.NonceRepository, repos.WalletRepository, ethClient))
	authGroup.GET("/jwt_valid", middleware.AuthOptional(), auth.ValidateJWT())
	authGroup.GET("/is_member", middleware.AuthOptional(), hasNFTs(repos.UserRepository, ethClient, membership.PremiumCards, membership.MembershipTierIDs))
	authGroup.POST("/logout", logout())

	// USER

	usersGroup.POST("/login", login(repos.UserRepository, repos.NonceRepository, repos.LoginRepository, ethClient))
	usersGroup.POST("/update/info", middleware.AuthRequired(repos.UserRepository, ethClient), updateUserInfo(repos.UserRepository, ethClient))
	usersGroup.POST("/update/addresses/add", middleware.AuthRequired(repos.UserRepository, ethClient), addUserAddress(repos.UserRepository, repos.WalletRepository, repos.NonceRepository, ethClient))
	usersGroup.POST("/update/addresses/remove", middleware.AuthRequired(repos.UserRepository, ethClient), removeAddresses(repos.UserRepository, repos.WalletRepository))
	usersGroup.GET("/get", middleware.AuthOptional(), getUser(repos.UserRepository))
	usersGroup.GET("/get/current", middleware.AuthOptional(), getCurrentUser(repos.UserRepository))
	usersGroup.GET("/membership", getMembershipTiersREST(repos.MembershipRepository, repos.UserRepository, repos.GalleryRepository, ethClient, ipfsClient, arweaveClient, storageClient))
	usersGroup.POST("/create", createUser(repos.UserRepository, repos.NonceRepository, repos.GalleryRepository, ethClient))
	usersGroup.GET("/previews", getNFTPreviews(repos.GalleryRepository, repos.UserRepository))
	usersGroup.POST("/merge", middleware.AuthRequired(repos.UserRepository, ethClient), mergeUsers(repos.UserRepository, repos.NonceRepository, repos.WalletRepository, mcProvider))

}

func tokenHandlersInit(parent *gin.RouterGroup, repos *persist.Repositories, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, mcProvider *multichain.Provider) {

	// AUTH

	authHandlersInitToken(parent, repos, ethClient, ipfsClient, arweaveClient, stg, mcProvider)

	// GALLERIES

	galleriesGroup := parent.Group("/galleries")

	galleriesGroup.GET("/get", middleware.AuthOptional(), getGalleryByIDToken(repos.GalleryTokenRepository, repos.TokenRepository, ipfsClient, ethClient))
	galleriesGroup.GET("/user_get", middleware.AuthOptional(), getGalleriesByUserIDToken(repos.GalleryTokenRepository, repos.TokenRepository, ipfsClient, ethClient))
	galleriesGroup.POST("/update", middleware.AuthRequired(repos.UserRepository, ethClient), updateGalleryToken(repos.GalleryTokenRepository))
	// COLLECTIONS

	collectionsGroup := parent.Group("/collections")

	collectionsGroup.GET("/get", middleware.AuthOptional(), getCollectionByIDToken(repos.CollectionTokenRepository, repos.TokenRepository, ipfsClient, ethClient))
	collectionsGroup.GET("/user_get", middleware.AuthOptional(), getCollectionsByUserIDToken(repos.CollectionTokenRepository, repos.TokenRepository, ipfsClient, ethClient))
	collectionsGroup.POST("/create", middleware.AuthRequired(repos.UserRepository, ethClient), createCollectionToken(repos.CollectionTokenRepository, repos.GalleryTokenRepository))
	collectionsGroup.POST("/delete", middleware.AuthRequired(repos.UserRepository, ethClient), deleteCollectionToken(repos.CollectionTokenRepository))
	collectionsGroup.POST("/update/info", middleware.AuthRequired(repos.UserRepository, ethClient), updateCollectionInfoToken(repos.CollectionTokenRepository))
	collectionsGroup.POST("/update/hidden", middleware.AuthRequired(repos.UserRepository, ethClient), updateCollectionHiddenToken(repos.CollectionTokenRepository))
	collectionsGroup.POST("/update/nfts", middleware.AuthRequired(repos.UserRepository, ethClient), updateCollectionTokensToken(repos.CollectionTokenRepository))

	// NFTS

	nftsGroup := parent.Group("/nfts")

	nftsGroup.GET("/get", middleware.AuthOptional(), getTokens(repos.TokenRepository, ipfsClient, ethClient))
	nftsGroup.GET("/user_get", middleware.AuthOptional(), getTokensForUser(repos.TokenRepository, ipfsClient, ethClient))
	nftsGroup.POST("/update", middleware.AuthRequired(repos.UserRepository, ethClient), updateTokenByID(repos.TokenRepository))
	nftsGroup.GET("/unassigned/get", middleware.AuthRequired(repos.UserRepository, ethClient), getUnassignedTokensForUser(repos.CollectionTokenRepository, repos.TokenRepository, ipfsClient, ethClient))
	nftsGroup.POST("/unassigned/refresh", middleware.AuthRequired(repos.UserRepository, ethClient), refreshUnassignedTokensForUser(repos.CollectionTokenRepository))
	nftsGroup.GET("/metadata/refresh", refreshMetadataForToken(repos.TokenRepository, ethClient, ipfsClient, arweaveClient, stg))

	communitiesGroup := parent.Group("/communities")

	communitiesGroup.GET("/get", middleware.AuthOptional(), getCommunity(repos.CommunityRepository))

	proxy := parent.Group("/proxy")
	proxy.GET("/snapshot", proxySnapshot(stg))

	parent.GET("/health", healthcheck())

}

func nftHandlersInit(parent *gin.RouterGroup, repos *persist.Repositories, ethClient *ethclient.Client, stg *storage.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, mcProvider *multichain.Provider) {

	// AUTH

	authHandlersInitNFT(parent, repos, ethClient, ipfsClient, arweaveClient, stg, mcProvider)

	// GALLERIES

	galleriesGroup := parent.Group("/galleries")

	galleriesGroup.GET("/get", middleware.AuthOptional(), getGalleryByID(repos.GalleryRepository))
	galleriesGroup.GET("/user_get", middleware.AuthOptional(), getGalleriesByUserID(repos.GalleryRepository))
	galleriesGroup.POST("/update", middleware.AuthRequired(repos.UserRepository, ethClient), updateGallery(repos.GalleryRepository, repos.BackupRepository))
	galleriesGroup.POST("/refresh", middleware.RateLimited(), refreshGallery(repos.GalleryRepository))

	// COLLECTIONS

	collectionsGroup := parent.Group("/collections")

	collectionsGroup.GET("/get", middleware.AuthOptional(), getCollectionByID(repos.CollectionRepository))
	collectionsGroup.GET("/user_get", middleware.AuthOptional(), getCollectionsByUserID(repos.CollectionRepository))
	collectionsGroup.POST("/create", middleware.AuthRequired(repos.UserRepository, ethClient), createCollection(repos.CollectionRepository, repos.GalleryRepository))
	collectionsGroup.POST("/delete", middleware.AuthRequired(repos.UserRepository, ethClient), deleteCollection(repos.CollectionRepository))
	collectionsGroup.POST("/update/info", middleware.AuthRequired(repos.UserRepository, ethClient), updateCollectionInfo(repos.CollectionRepository))
	collectionsGroup.POST("/update/hidden", middleware.AuthRequired(repos.UserRepository, ethClient), updateCollectionHidden(repos.CollectionRepository))
	collectionsGroup.POST("/update/nfts", middleware.AuthRequired(repos.UserRepository, ethClient), updateCollectionNfts(repos.CollectionRepository, repos.GalleryRepository, repos.BackupRepository))

	// NFTS

	nftsGroup := parent.Group("/nfts")

	nftsGroup.GET("/get", middleware.AuthOptional(), getNftByID(repos.NftRepository))
	nftsGroup.GET("/user_get", middleware.AuthOptional(), getNftsForUser(repos.NftRepository))
	nftsGroup.POST("/update", middleware.AuthRequired(repos.UserRepository, ethClient), updateNftByID(repos.NftRepository))
	communitiesGroup := parent.Group("/communities")

	communitiesGroup.GET("/get", middleware.AuthOptional(), getCommunity(repos.CommunityRepository))

	proxy := parent.Group("/proxy")
	proxy.GET("/snapshot", proxySnapshot(stg))

	parent.GET("/health", healthcheck())

}
