package server

import (
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/eth"
)

func handlersInit(router *gin.Engine, repos *repositories, ethClient *eth.Client, ipfsClient *shell.Shell) *gin.Engine {

	apiGroupV1 := router.Group("/glry/v1")

	nftHandlersInit(apiGroupV1, repos, ethClient)
	// tokenHandlersInit(apiGroupV1, repos, ethClient, ipfsClient)

	return router
}

func authHandlersInitToken(parent *gin.RouterGroup, repos *repositories, ethClient *eth.Client) {

	usersGroup := parent.Group("/users")

	authGroup := parent.Group("/auth")

	// AUTH
	authGroup.GET("/get_preflight", jwtOptional(), getAuthPreflight(repos.userRepository, repos.nonceRepository, ethClient))
	authGroup.GET("/jwt_valid", jwtOptional(), validateJwt())

	// USER

	usersGroup.POST("/login", login(repos.userRepository, repos.nonceRepository, repos.loginRepository))
	usersGroup.POST("/update/info", jwtRequired(), updateUserInfo(repos.userRepository, ethClient))
	usersGroup.POST("/update/addresses/add", jwtRequired(), addUserAddress(repos.userRepository, repos.nonceRepository))
	usersGroup.POST("/update/addresses/remove", jwtRequired(), removeAddressesToken(repos.userRepository, repos.collectionTokenRepository))
	usersGroup.GET("/get", jwtOptional(), getUser(repos.userRepository))
	usersGroup.POST("/create", createUserToken(repos.userRepository, repos.nonceRepository, repos.galleryTokenRepository))

}

func authHandlersInitNFT(parent *gin.RouterGroup, repos *repositories, ethClient *eth.Client) {

	usersGroup := parent.Group("/users")

	authGroup := parent.Group("/auth")

	// AUTH
	authGroup.GET("/get_preflight", jwtOptional(), getAuthPreflight(repos.userRepository, repos.nonceRepository, ethClient))
	authGroup.GET("/jwt_valid", jwtOptional(), validateJwt())

	// USER

	usersGroup.POST("/login", login(repos.userRepository, repos.nonceRepository, repos.loginRepository))
	usersGroup.POST("/update/info", jwtRequired(), updateUserInfo(repos.userRepository, ethClient))
	usersGroup.POST("/update/addresses/add", jwtRequired(), addUserAddress(repos.userRepository, repos.nonceRepository))
	usersGroup.POST("/update/addresses/remove", jwtRequired(), removeAddresses(repos.userRepository, repos.collectionRepository))
	usersGroup.GET("/get", jwtOptional(), getUser(repos.userRepository))
	usersGroup.POST("/create", createUser(repos.userRepository, repos.nonceRepository, repos.galleryRepository))

}

func tokenHandlersInit(parent *gin.RouterGroup, repos *repositories, ethClient *eth.Client, ipfsClient *shell.Shell) {

	// AUTH

	authHandlersInitToken(parent, repos, ethClient)

	// GALLERIES

	galleriesGroup := parent.Group("/galleries")

	galleriesGroup.GET("/get", jwtOptional(), getGalleryByIDToken(repos.galleryTokenRepository, repos.tokenRepository, ipfsClient))
	galleriesGroup.GET("/user_get", jwtOptional(), getGalleriesByUserIDToken(repos.galleryTokenRepository, repos.tokenRepository, ipfsClient))
	galleriesGroup.POST("/update", jwtRequired(), updateGalleryToken(repos.galleryTokenRepository))

	// COLLECTIONS

	collectionsGroup := parent.Group("/collections")

	collectionsGroup.GET("/get", jwtOptional(), getCollectionByIDToken(repos.collectionTokenRepository, repos.tokenRepository, ipfsClient))
	collectionsGroup.GET("/user_get", jwtOptional(), getCollectionsByUserIDToken(repos.collectionTokenRepository, repos.tokenRepository, ipfsClient))
	collectionsGroup.POST("/create", jwtRequired(), createCollectionToken(repos.collectionTokenRepository, repos.galleryTokenRepository))
	collectionsGroup.POST("/delete", jwtRequired(), deleteCollectionToken(repos.collectionTokenRepository))
	collectionsGroup.POST("/update/info", jwtRequired(), updateCollectionInfoToken(repos.collectionTokenRepository))
	collectionsGroup.POST("/update/hidden", jwtRequired(), updateCollectionHiddenToken(repos.collectionTokenRepository))
	collectionsGroup.POST("/update/nfts", jwtRequired(), updateCollectionTokensToken(repos.collectionTokenRepository))

	// NFTS

	nftsGroup := parent.Group("/nfts")

	nftsGroup.GET("/get", jwtOptional(), getTokenByID(repos.tokenRepository, ipfsClient))
	nftsGroup.GET("/user_get", jwtOptional(), getTokensForUser(repos.tokenRepository, ipfsClient))
	nftsGroup.POST("/update", jwtRequired(), updateTokenByID(repos.tokenRepository))
	nftsGroup.GET("/unassigned/get", jwtRequired(), getUnassignedTokensForUser(repos.collectionTokenRepository, repos.tokenRepository, ipfsClient))
	nftsGroup.POST("/unassigned/refresh", jwtRequired(), refreshUnassignedTokensForUser(repos.collectionTokenRepository))

	parent.GET("/health", healthcheck())

}

func nftHandlersInit(parent *gin.RouterGroup, repos *repositories, ethClient *eth.Client) {

	// AUTH

	authHandlersInitNFT(parent, repos, ethClient)

	// GALLERIES

	galleriesGroup := parent.Group("/galleries")

	galleriesGroup.GET("/get", jwtOptional(), getGalleryByID(repos.galleryRepository))
	galleriesGroup.GET("/user_get", jwtOptional(), getGalleriesByUserID(repos.galleryRepository))
	galleriesGroup.POST("/update", jwtRequired(), updateGallery(repos.galleryRepository, repos.backupRepository))

	// COLLECTIONS

	collectionsGroup := parent.Group("/collections")

	collectionsGroup.GET("/get", jwtOptional(), getCollectionByID(repos.collectionRepository))
	collectionsGroup.GET("/user_get", jwtOptional(), getCollectionsByUserID(repos.collectionRepository))
	collectionsGroup.POST("/create", jwtRequired(), createCollection(repos.collectionRepository, repos.galleryRepository))
	collectionsGroup.POST("/delete", jwtRequired(), deleteCollection(repos.collectionRepository))
	collectionsGroup.POST("/update/info", jwtRequired(), updateCollectionInfo(repos.collectionRepository))
	collectionsGroup.POST("/update/hidden", jwtRequired(), updateCollectionHidden(repos.collectionRepository))
	collectionsGroup.POST("/update/nfts", jwtRequired(), updateCollectionNfts(repos.collectionRepository, repos.galleryRepository, repos.backupRepository))

	// NFTS

	nftsGroup := parent.Group("/nfts")

	nftsGroup.GET("/get", jwtOptional(), getNftByID(repos.nftRepository))
	nftsGroup.GET("/user_get", jwtOptional(), getNftsForUser(repos.nftRepository))
	nftsGroup.GET("/opensea_get", rateLimited(), jwtRequired(), getNftsFromOpensea(repos.nftRepository, repos.userRepository, repos.collectionRepository, repos.historyRepository))
	nftsGroup.POST("/update", jwtRequired(), updateNftByID(repos.nftRepository))
	nftsGroup.GET("/unassigned/get", jwtRequired(), getUnassignedNftsForUser(repos.collectionRepository))
	nftsGroup.POST("/unassigned/refresh", jwtRequired(), refreshUnassignedNftsForUser(repos.collectionRepository))

	parent.GET("/health", healthcheck())

}
