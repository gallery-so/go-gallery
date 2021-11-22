package server

import (
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/eth"
	"github.com/mikeydub/go-gallery/middleware"
)

func handlersInit(router *gin.Engine, repos *repositories, ethClient *eth.Client, ipfsClient *shell.Shell) *gin.Engine {

	apiGroupV1 := router.Group("/glry/v1")
	apiGroupV2 := router.Group("/glry/v2")

	nftHandlersInit(apiGroupV1, repos, ethClient)
	tokenHandlersInit(apiGroupV2, repos, ethClient, ipfsClient)

	return router
}

func authHandlersInitToken(parent *gin.RouterGroup, repos *repositories, ethClient *eth.Client) {

	usersGroup := parent.Group("/users")

	authGroup := parent.Group("/auth")

	// AUTH
	authGroup.GET("/get_preflight", middleware.JWTOptional(), getAuthPreflight(repos.userRepository, repos.nonceRepository, ethClient))
	authGroup.GET("/jwt_valid", middleware.JWTOptional(), middleware.ValidateJWT())
	authGroup.GET("/is_member", middleware.JWTOptional(), hasNFTs(repos.userRepository, ethClient, middleware.RequiredNFTs))

	// USER

	usersGroup.POST("/login", login(repos.userRepository, repos.nonceRepository, repos.loginRepository))
	usersGroup.POST("/update/info", middleware.JWTRequired(repos.userRepository, ethClient), updateUserInfo(repos.userRepository, ethClient))
	usersGroup.POST("/update/addresses/add", middleware.JWTRequired(repos.userRepository, ethClient), addUserAddress(repos.userRepository, repos.nonceRepository))
	usersGroup.POST("/update/addresses/remove", middleware.JWTRequired(repos.userRepository, ethClient), removeAddressesToken(repos.userRepository, repos.collectionTokenRepository))
	usersGroup.GET("/get", middleware.JWTOptional(), getUser(repos.userRepository))
	usersGroup.GET("/membership", getMembershipTiers(repos.membershipRepository, repos.userRepository, ethClient))
	usersGroup.POST("/create", createUserToken(repos.userRepository, repos.nonceRepository, repos.galleryTokenRepository))

}

func authHandlersInitNFT(parent *gin.RouterGroup, repos *repositories, ethClient *eth.Client) {

	usersGroup := parent.Group("/users")

	authGroup := parent.Group("/auth")

	// AUTH
	authGroup.GET("/get_preflight", middleware.JWTOptional(), getAuthPreflight(repos.userRepository, repos.nonceRepository, ethClient))
	authGroup.GET("/jwt_valid", middleware.JWTOptional(), middleware.ValidateJWT())
	authGroup.GET("/is_member", middleware.JWTOptional(), hasNFTs(repos.userRepository, ethClient, middleware.RequiredNFTs))

	// USER

	usersGroup.POST("/login", login(repos.userRepository, repos.nonceRepository, repos.loginRepository))
	usersGroup.POST("/update/info", middleware.JWTRequired(repos.userRepository, ethClient), updateUserInfo(repos.userRepository, ethClient))
	usersGroup.POST("/update/addresses/add", middleware.JWTRequired(repos.userRepository, ethClient), addUserAddress(repos.userRepository, repos.nonceRepository))
	usersGroup.POST("/update/addresses/remove", middleware.JWTRequired(repos.userRepository, ethClient), removeAddresses(repos.userRepository, repos.collectionRepository))
	usersGroup.GET("/get", middleware.JWTOptional(), getUser(repos.userRepository))
	usersGroup.GET("/membership", getMembershipTiers(repos.membershipRepository, repos.userRepository, ethClient))
	usersGroup.POST("/create", createUser(repos.userRepository, repos.nonceRepository, repos.galleryRepository))

}

func tokenHandlersInit(parent *gin.RouterGroup, repos *repositories, ethClient *eth.Client, ipfsClient *shell.Shell) {

	// AUTH

	authHandlersInitToken(parent, repos, ethClient)

	// GALLERIES

	galleriesGroup := parent.Group("/galleries")

	galleriesGroup.GET("/get", middleware.JWTOptional(), getGalleryByIDToken(repos.galleryTokenRepository, repos.tokenRepository, ipfsClient, ethClient.EthClient))
	galleriesGroup.GET("/user_get", middleware.JWTOptional(), getGalleriesByUserIDToken(repos.galleryTokenRepository, repos.tokenRepository, ipfsClient, ethClient.EthClient))
	galleriesGroup.POST("/update", middleware.JWTRequired(repos.userRepository, ethClient), updateGalleryToken(repos.galleryTokenRepository))

	// COLLECTIONS

	collectionsGroup := parent.Group("/collections")

	collectionsGroup.GET("/get", middleware.JWTOptional(), getCollectionByIDToken(repos.collectionTokenRepository, repos.tokenRepository, ipfsClient, ethClient.EthClient))
	collectionsGroup.GET("/user_get", middleware.JWTOptional(), getCollectionsByUserIDToken(repos.collectionTokenRepository, repos.tokenRepository, ipfsClient, ethClient.EthClient))
	collectionsGroup.POST("/create", middleware.JWTRequired(repos.userRepository, ethClient), createCollectionToken(repos.collectionTokenRepository, repos.galleryTokenRepository))
	collectionsGroup.POST("/delete", middleware.JWTRequired(repos.userRepository, ethClient), deleteCollectionToken(repos.collectionTokenRepository))
	collectionsGroup.POST("/update/info", middleware.JWTRequired(repos.userRepository, ethClient), updateCollectionInfoToken(repos.collectionTokenRepository))
	collectionsGroup.POST("/update/hidden", middleware.JWTRequired(repos.userRepository, ethClient), updateCollectionHiddenToken(repos.collectionTokenRepository))
	collectionsGroup.POST("/update/nfts", middleware.JWTRequired(repos.userRepository, ethClient), updateCollectionTokensToken(repos.collectionTokenRepository))

	// NFTS

	nftsGroup := parent.Group("/nfts")

	nftsGroup.GET("/get", middleware.JWTOptional(), getTokens(repos.tokenRepository, ipfsClient, ethClient.EthClient))
	nftsGroup.GET("/user_get", middleware.JWTOptional(), getTokensForUser(repos.tokenRepository, ipfsClient, ethClient.EthClient))
	nftsGroup.POST("/update", middleware.JWTRequired(repos.userRepository, ethClient), updateTokenByID(repos.tokenRepository))
	nftsGroup.GET("/unassigned/get", middleware.JWTRequired(repos.userRepository, ethClient), getUnassignedTokensForUser(repos.collectionTokenRepository, repos.tokenRepository, ipfsClient, ethClient.EthClient))
	nftsGroup.POST("/unassigned/refresh", middleware.JWTRequired(repos.userRepository, ethClient), refreshUnassignedTokensForUser(repos.collectionTokenRepository))

	parent.GET("/health", healthcheck())

}

func nftHandlersInit(parent *gin.RouterGroup, repos *repositories, ethClient *eth.Client) {

	// AUTH

	authHandlersInitNFT(parent, repos, ethClient)

	// GALLERIES

	galleriesGroup := parent.Group("/galleries")

	galleriesGroup.GET("/get", middleware.JWTOptional(), getGalleryByID(repos.galleryRepository))
	galleriesGroup.GET("/user_get", middleware.JWTOptional(), getGalleriesByUserID(repos.galleryRepository))
	galleriesGroup.POST("/update", middleware.JWTRequired(repos.userRepository, ethClient), updateGallery(repos.galleryRepository, repos.backupRepository))

	// COLLECTIONS

	collectionsGroup := parent.Group("/collections")

	collectionsGroup.GET("/get", middleware.JWTOptional(), getCollectionByID(repos.collectionRepository))
	collectionsGroup.GET("/user_get", middleware.JWTOptional(), getCollectionsByUserID(repos.collectionRepository))
	collectionsGroup.POST("/create", middleware.JWTRequired(repos.userRepository, ethClient), createCollection(repos.collectionRepository, repos.galleryRepository))
	collectionsGroup.POST("/delete", middleware.JWTRequired(repos.userRepository, ethClient), deleteCollection(repos.collectionRepository))
	collectionsGroup.POST("/update/info", middleware.JWTRequired(repos.userRepository, ethClient), updateCollectionInfo(repos.collectionRepository))
	collectionsGroup.POST("/update/hidden", middleware.JWTRequired(repos.userRepository, ethClient), updateCollectionHidden(repos.collectionRepository))
	collectionsGroup.POST("/update/nfts", middleware.JWTRequired(repos.userRepository, ethClient), updateCollectionNfts(repos.collectionRepository, repos.galleryRepository, repos.backupRepository))

	// NFTS

	nftsGroup := parent.Group("/nfts")

	nftsGroup.GET("/get", middleware.JWTOptional(), getNftByID(repos.nftRepository))
	nftsGroup.GET("/user_get", middleware.JWTOptional(), getNftsForUser(repos.nftRepository))
	nftsGroup.GET("/opensea/get", middleware.JWTRequired(repos.userRepository, ethClient), getNftsFromOpensea(repos.nftRepository, repos.userRepository, repos.collectionRepository, repos.historyRepository))
	nftsGroup.POST("/opensea/refresh", middleware.RateLimited(), middleware.JWTRequired(repos.userRepository, ethClient), refreshOpenseaNFTs(repos.nftRepository, repos.userRepository))
	nftsGroup.POST("/update", middleware.JWTRequired(repos.userRepository, ethClient), updateNftByID(repos.nftRepository))
	nftsGroup.GET("/unassigned/get", middleware.JWTRequired(repos.userRepository, ethClient), getUnassignedNftsForUser(repos.collectionRepository))
	nftsGroup.POST("/unassigned/refresh", middleware.JWTRequired(repos.userRepository, ethClient), refreshUnassignedNftsForUser(repos.collectionRepository))

	parent.GET("/health", healthcheck())

}
