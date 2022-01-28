package server

import (
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/membership"
	"github.com/mikeydub/go-gallery/service/pubsub"
)

func handlersInit(router *gin.Engine, repos *repositories, ethClient *ethclient.Client, ipfsClient *shell.Shell, psub pubsub.PubSub) *gin.Engine {

	apiGroupV1 := router.Group("/glry/v1")
	apiGroupV2 := router.Group("/glry/v2")

	nftHandlersInit(apiGroupV1, repos, ethClient, psub)
	tokenHandlersInit(apiGroupV2, repos, ethClient, ipfsClient, psub)

	return router
}

func authHandlersInitToken(parent *gin.RouterGroup, repos *repositories, ethClient *ethclient.Client, psub pubsub.PubSub) {

	usersGroup := parent.Group("/users")

	authGroup := parent.Group("/auth")

	// AUTH
	authGroup.GET("/get_preflight", middleware.AuthOptional(), getAuthPreflight(repos.userRepository, repos.nonceRepository, ethClient))
	authGroup.GET("/jwt_valid", middleware.AuthOptional(), auth.ValidateJWT())
	authGroup.GET("/is_member", middleware.AuthOptional(), hasNFTs(repos.userRepository, ethClient, membership.PremiumCards, membership.MembershipTierIDs))
	authGroup.POST("/logout", logout())

	// USER

	usersGroup.POST("/login", login(repos.userRepository, repos.nonceRepository, repos.loginRepository, ethClient))
	usersGroup.POST("/update/info", middleware.AuthRequired(repos.userRepository, ethClient), updateUserInfo(repos.userRepository, ethClient))
	usersGroup.POST("/update/addresses/add", middleware.AuthRequired(repos.userRepository, ethClient), addUserAddress(repos.userRepository, repos.nonceRepository, ethClient, psub))
	usersGroup.POST("/update/addresses/remove", middleware.AuthRequired(repos.userRepository, ethClient), removeAddressesToken(repos.userRepository, repos.collectionTokenRepository))
	usersGroup.GET("/get", middleware.AuthOptional(), getUser(repos.userRepository))
	usersGroup.GET("/get/current", middleware.AuthOptional(), getCurrentUser(repos.userRepository))
	usersGroup.GET("/membership", getMembershipTiersToken(repos.membershipRepository, repos.userRepository, repos.tokenRepository, repos.galleryTokenRepository, ethClient))
	usersGroup.POST("/create", createUserToken(repos.userRepository, repos.nonceRepository, repos.galleryTokenRepository, psub, ethClient))

}

func authHandlersInitNFT(parent *gin.RouterGroup, repos *repositories, ethClient *ethclient.Client, psub pubsub.PubSub) {

	usersGroup := parent.Group("/users")

	authGroup := parent.Group("/auth")

	// AUTH
	authGroup.GET("/get_preflight", middleware.AuthOptional(), getAuthPreflight(repos.userRepository, repos.nonceRepository, ethClient))
	authGroup.GET("/jwt_valid", middleware.AuthOptional(), auth.ValidateJWT())
	authGroup.GET("/is_member", middleware.AuthOptional(), hasNFTs(repos.userRepository, ethClient, membership.PremiumCards, membership.MembershipTierIDs))
	authGroup.POST("/logout", logout())

	// USER

	usersGroup.POST("/login", login(repos.userRepository, repos.nonceRepository, repos.loginRepository, ethClient))
	usersGroup.POST("/update/info", middleware.AuthRequired(repos.userRepository, ethClient), updateUserInfo(repos.userRepository, ethClient))
	usersGroup.POST("/update/addresses/add", middleware.AuthRequired(repos.userRepository, ethClient), addUserAddress(repos.userRepository, repos.nonceRepository, ethClient, psub))
	usersGroup.POST("/update/addresses/remove", middleware.AuthRequired(repos.userRepository, ethClient), removeAddresses(repos.userRepository, repos.collectionRepository))
	usersGroup.GET("/get", middleware.AuthOptional(), getUser(repos.userRepository))
	usersGroup.GET("/get/current", middleware.AuthOptional(), getCurrentUser(repos.userRepository))
	usersGroup.GET("/membership", getMembershipTiers(repos.membershipRepository, repos.userRepository, repos.galleryRepository, ethClient))
	usersGroup.POST("/create", createUser(repos.userRepository, repos.nonceRepository, repos.galleryRepository, psub, ethClient))

}

func tokenHandlersInit(parent *gin.RouterGroup, repos *repositories, ethClient *ethclient.Client, ipfsClient *shell.Shell, psub pubsub.PubSub) {

	// AUTH

	authHandlersInitToken(parent, repos, ethClient, psub)

	// GALLERIES

	galleriesGroup := parent.Group("/galleries")

	galleriesGroup.GET("/get", middleware.AuthOptional(), getGalleryByIDToken(repos.galleryTokenRepository, repos.tokenRepository, ipfsClient, ethClient))
	galleriesGroup.GET("/user_get", middleware.AuthOptional(), getGalleriesByUserIDToken(repos.galleryTokenRepository, repos.tokenRepository, ipfsClient, ethClient))
	galleriesGroup.POST("/update", middleware.AuthRequired(repos.userRepository, ethClient), updateGalleryToken(repos.galleryTokenRepository))
	// COLLECTIONS

	collectionsGroup := parent.Group("/collections")

	collectionsGroup.GET("/get", middleware.AuthOptional(), getCollectionByIDToken(repos.collectionTokenRepository, repos.tokenRepository, ipfsClient, ethClient))
	collectionsGroup.GET("/user_get", middleware.AuthOptional(), getCollectionsByUserIDToken(repos.collectionTokenRepository, repos.tokenRepository, ipfsClient, ethClient))
	collectionsGroup.POST("/create", middleware.AuthRequired(repos.userRepository, ethClient), createCollectionToken(repos.collectionTokenRepository, repos.galleryTokenRepository))
	collectionsGroup.POST("/delete", middleware.AuthRequired(repos.userRepository, ethClient), deleteCollectionToken(repos.collectionTokenRepository))
	collectionsGroup.POST("/update/info", middleware.AuthRequired(repos.userRepository, ethClient), updateCollectionInfoToken(repos.collectionTokenRepository))
	collectionsGroup.POST("/update/hidden", middleware.AuthRequired(repos.userRepository, ethClient), updateCollectionHiddenToken(repos.collectionTokenRepository))
	collectionsGroup.POST("/update/nfts", middleware.AuthRequired(repos.userRepository, ethClient), updateCollectionTokensToken(repos.collectionTokenRepository))

	// NFTS

	nftsGroup := parent.Group("/nfts")

	nftsGroup.GET("/get", middleware.AuthOptional(), getTokens(repos.tokenRepository, ipfsClient, ethClient))
	nftsGroup.GET("/user_get", middleware.AuthOptional(), getTokensForUser(repos.tokenRepository, ipfsClient, ethClient))
	nftsGroup.POST("/update", middleware.AuthRequired(repos.userRepository, ethClient), updateTokenByID(repos.tokenRepository))
	nftsGroup.GET("/unassigned/get", middleware.AuthRequired(repos.userRepository, ethClient), getUnassignedTokensForUser(repos.collectionTokenRepository, repos.tokenRepository, ipfsClient, ethClient))
	nftsGroup.POST("/unassigned/refresh", middleware.AuthRequired(repos.userRepository, ethClient), refreshUnassignedTokensForUser(repos.collectionTokenRepository))

	parent.GET("/health", healthcheck())

}

func nftHandlersInit(parent *gin.RouterGroup, repos *repositories, ethClient *ethclient.Client, psub pubsub.PubSub) {

	// AUTH

	authHandlersInitNFT(parent, repos, ethClient, psub)

	// GALLERIES

	galleriesGroup := parent.Group("/galleries")

	galleriesGroup.GET("/get", middleware.AuthOptional(), getGalleryByID(repos.galleryRepository))
	galleriesGroup.GET("/user_get", middleware.AuthOptional(), getGalleriesByUserID(repos.galleryRepository))
	galleriesGroup.POST("/update", middleware.AuthRequired(repos.userRepository, ethClient), updateGallery(repos.galleryRepository, repos.backupRepository))
	galleriesGroup.POST("/refresh", middleware.RateLimited(), refreshGallery(repos.galleryRepository))

	// COLLECTIONS

	collectionsGroup := parent.Group("/collections")

	collectionsGroup.GET("/get", middleware.AuthOptional(), getCollectionByID(repos.collectionRepository))
	collectionsGroup.GET("/user_get", middleware.AuthOptional(), getCollectionsByUserID(repos.collectionRepository))
	collectionsGroup.POST("/create", middleware.AuthRequired(repos.userRepository, ethClient), createCollection(repos.collectionRepository, repos.galleryRepository))
	collectionsGroup.POST("/delete", middleware.AuthRequired(repos.userRepository, ethClient), deleteCollection(repos.collectionRepository))
	collectionsGroup.POST("/update/info", middleware.AuthRequired(repos.userRepository, ethClient), updateCollectionInfo(repos.collectionRepository))
	collectionsGroup.POST("/update/hidden", middleware.AuthRequired(repos.userRepository, ethClient), updateCollectionHidden(repos.collectionRepository))
	collectionsGroup.POST("/update/nfts", middleware.AuthRequired(repos.userRepository, ethClient), updateCollectionNfts(repos.collectionRepository, repos.galleryRepository, repos.backupRepository))

	// NFTS

	nftsGroup := parent.Group("/nfts")

	nftsGroup.GET("/get", middleware.AuthOptional(), getNftByID(repos.nftRepository))
	nftsGroup.GET("/user_get", middleware.AuthOptional(), getNftsForUser(repos.nftRepository))
	nftsGroup.GET("/opensea/get", middleware.AuthRequired(repos.userRepository, ethClient), getNftsFromOpensea(repos.nftRepository, repos.userRepository, repos.collectionRepository))
	nftsGroup.POST("/opensea/refresh", middleware.AuthRequired(repos.userRepository, ethClient), refreshOpenseaNFTs(repos.nftRepository, repos.userRepository))
	nftsGroup.POST("/update", middleware.AuthRequired(repos.userRepository, ethClient), updateNftByID(repos.nftRepository))
	nftsGroup.GET("/unassigned/get", middleware.AuthRequired(repos.userRepository, ethClient), getUnassignedNftsForUser(repos.collectionRepository))
	nftsGroup.POST("/unassigned/refresh", middleware.AuthRequired(repos.userRepository, ethClient), refreshUnassignedNftsForUser(repos.collectionRepository))

	parent.GET("/health", healthcheck())

}
