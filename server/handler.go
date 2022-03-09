package server

import (
	"cloud.google.com/go/storage"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/graphql/generated"
	graphql "github.com/mikeydub/go-gallery/graphql/resolver"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/membership"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/pubsub"
	"github.com/spf13/viper"
)

func handlersInit(router *gin.Engine, repos *persist.Repositories, ethClient *ethclient.Client, ipfsClient *shell.Shell, stg *storage.Client, psub pubsub.PubSub) *gin.Engine {

	apiGroupV1 := router.Group("/glry/v1")
	apiGroupV2 := router.Group("/glry/v2")
	graphqlGroup := router.Group("/glry/graphql")

	nftHandlersInit(apiGroupV1, repos, ethClient, stg, psub)
	tokenHandlersInit(apiGroupV2, repos, ethClient, ipfsClient, stg, psub)
	graphqlHandlersInit(graphqlGroup, repos, ethClient, psub)

	return router
}

func graphqlHandlersInit(parent *gin.RouterGroup, repos *persist.Repositories, ethClient *ethclient.Client, pubsub pubsub.PubSub) {
	parent.POST("/query", middleware.AddAuthToContext(), graphqlHandler(repos, ethClient, pubsub))

	if viper.GetString("ENV") != "production" {
		// TODO: Consider completely disabling introspection in production
		parent.GET("/playground", graphqlPlaygroundHandler())
	}
}

func graphqlHandler(repos *persist.Repositories, ethClient *ethclient.Client, pubsub pubsub.PubSub) gin.HandlerFunc {
	// TODO: Resolver probably doesn't need repos or ethClient once the publicAPI is done
	config := generated.Config{Resolvers: &graphql.Resolver{Repos: repos, EthClient: ethClient}}
	config.Directives.AuthRequired = graphql.AuthRequiredDirectiveHandler(ethClient)

	h := handler.NewDefaultServer(generated.NewExecutableSchema(config))

	return func(c *gin.Context) {
		// TODO: Remove dataloader here
		dataloader.AddTo(c, repos)
		publicapi.AddTo(c, repos, ethClient, pubsub)
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

func authHandlersInitToken(parent *gin.RouterGroup, repos *persist.Repositories, ethClient *ethclient.Client, psub pubsub.PubSub) {

	usersGroup := parent.Group("/users")

	authGroup := parent.Group("/auth")

	// AUTH
	authGroup.GET("/get_preflight", middleware.AuthOptional(), getAuthPreflight(repos.UserRepository, repos.NonceRepository, ethClient))
	authGroup.GET("/jwt_valid", middleware.AuthOptional(), auth.ValidateJWT())
	authGroup.GET("/is_member", middleware.AuthOptional(), hasNFTs(repos.UserRepository, ethClient, membership.PremiumCards, membership.MembershipTierIDs))
	authGroup.POST("/logout", logout())

	// USER

	usersGroup.POST("/login", login(repos.UserRepository, repos.NonceRepository, repos.LoginRepository, ethClient))
	usersGroup.POST("/update/info", middleware.AuthRequired(repos.UserRepository, ethClient), updateUserInfo(repos.UserRepository, ethClient))
	usersGroup.POST("/update/addresses/add", middleware.AuthRequired(repos.UserRepository, ethClient), addUserAddress(repos.UserRepository, repos.NonceRepository, ethClient, psub))
	usersGroup.POST("/update/addresses/remove", middleware.AuthRequired(repos.UserRepository, ethClient), removeAddressesToken(repos.UserRepository))
	usersGroup.GET("/get", middleware.AuthOptional(), getUser(repos.UserRepository))
	usersGroup.GET("/get/current", middleware.AuthOptional(), getCurrentUser(repos.UserRepository))
	usersGroup.GET("/membership", getMembershipTiersToken(repos.MembershipRepository, repos.UserRepository, repos.TokenRepository, repos.GalleryTokenRepository, ethClient))
	usersGroup.POST("/create", createUserToken(repos.UserRepository, repos.NonceRepository, repos.GalleryTokenRepository, psub, ethClient))
	usersGroup.GET("/previews", getNFTPreviewsToken(repos.GalleryTokenRepository, repos.UserRepository))
	usersGroup.POST("/merge", middleware.AuthRequired(repos.UserRepository, ethClient), mergeUsers(repos.UserRepository, repos.NonceRepository, ethClient))
}

func authHandlersInitNFT(parent *gin.RouterGroup, repos *persist.Repositories, ethClient *ethclient.Client, psub pubsub.PubSub) {

	usersGroup := parent.Group("/users")

	authGroup := parent.Group("/auth")

	// AUTH
	authGroup.GET("/get_preflight", middleware.AuthOptional(), getAuthPreflight(repos.UserRepository, repos.NonceRepository, ethClient))
	authGroup.GET("/jwt_valid", middleware.AuthOptional(), auth.ValidateJWT())
	authGroup.GET("/is_member", middleware.AuthOptional(), hasNFTs(repos.UserRepository, ethClient, membership.PremiumCards, membership.MembershipTierIDs))
	authGroup.POST("/logout", logout())

	// USER

	usersGroup.POST("/login", login(repos.UserRepository, repos.NonceRepository, repos.LoginRepository, ethClient))
	usersGroup.POST("/update/info", middleware.AuthRequired(repos.UserRepository, ethClient), updateUserInfo(repos.UserRepository, ethClient))
	usersGroup.POST("/update/addresses/add", middleware.AuthRequired(repos.UserRepository, ethClient), addUserAddress(repos.UserRepository, repos.NonceRepository, ethClient, psub))
	usersGroup.POST("/update/addresses/remove", middleware.AuthRequired(repos.UserRepository, ethClient), removeAddresses(repos.UserRepository))
	usersGroup.GET("/get", middleware.AuthOptional(), getUser(repos.UserRepository))
	usersGroup.GET("/get/current", middleware.AuthOptional(), getCurrentUser(repos.UserRepository))
	usersGroup.GET("/membership", getMembershipTiersREST(repos.MembershipRepository, repos.UserRepository, repos.GalleryRepository, ethClient))
	usersGroup.POST("/create", createUser(repos.UserRepository, repos.NonceRepository, repos.GalleryRepository, psub, ethClient))
	usersGroup.GET("/previews", getNFTPreviews(repos.GalleryRepository, repos.UserRepository))
	usersGroup.POST("/merge", middleware.AuthRequired(repos.UserRepository, ethClient), mergeUsers(repos.UserRepository, repos.NonceRepository, ethClient))
	usersGroup.GET("/communities", getAllCommunities(repos.CommunityRepository, repos.NftRepository, repos.GalleryRepository, repos.UserRepository))
	usersGroup.GET("/communities/:address", getCommunity(repos.CommunityRepository, repos.NftRepository, repos.GalleryRepository, repos.UserRepository))

}

func tokenHandlersInit(parent *gin.RouterGroup, repos *persist.Repositories, ethClient *ethclient.Client, ipfsClient *shell.Shell, stg *storage.Client, psub pubsub.PubSub) {

	// AUTH

	authHandlersInitToken(parent, repos, ethClient, psub)

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

	proxy := parent.Group("/proxy")
	proxy.GET("/snapshot", proxySnapshot(stg))

	parent.GET("/health", healthcheck())

}

func nftHandlersInit(parent *gin.RouterGroup, repos *persist.Repositories, ethClient *ethclient.Client, stg *storage.Client, psub pubsub.PubSub) {

	// AUTH

	authHandlersInitNFT(parent, repos, ethClient, psub)

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
	nftsGroup.GET("/opensea/get", middleware.AuthRequired(repos.UserRepository, ethClient), getNftsFromOpensea(repos.NftRepository, repos.UserRepository, repos.CollectionRepository))
	nftsGroup.POST("/opensea/refresh", middleware.AuthRequired(repos.UserRepository, ethClient), refreshOpenseaNFTsREST(repos.NftRepository, repos.UserRepository))
	nftsGroup.POST("/update", middleware.AuthRequired(repos.UserRepository, ethClient), updateNftByID(repos.NftRepository))
	nftsGroup.GET("/unassigned/get", middleware.AuthRequired(repos.UserRepository, ethClient), getUnassignedNftsForUser(repos.CollectionRepository))
	nftsGroup.POST("/unassigned/refresh", middleware.AuthRequired(repos.UserRepository, ethClient), refreshUnassignedNftsForUser(repos.CollectionRepository))

	proxy := parent.Group("/proxy")
	proxy.GET("/snapshot", proxySnapshot(stg))

	parent.GET("/health", healthcheck())

}
