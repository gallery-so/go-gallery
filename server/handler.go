package server

import (
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/eth"
)

func handlersInit(router *gin.Engine, ethClient *eth.Client, ipfsClient *shell.Shell) *gin.Engine {

	repos := newRepos()

	apiGroupV1 := router.Group("/glry/v1")

	// AUTH_HANDLERS
	authHandlersInit(apiGroupV1, repos, ethClient)

	// GALLERIES

	galleriesGroup := apiGroupV1.Group("/galleries")

	galleriesGroup.GET("/get", jwtOptional(), getGalleryByID(repos.galleryRepository, repos.tokenRepository, ipfsClient))
	galleriesGroup.GET("/user_get", jwtOptional(), getGalleriesByUserID(repos.galleryRepository, repos.tokenRepository, ipfsClient))
	galleriesGroup.POST("/update", jwtRequired(), updateGallery(repos.galleryRepository))

	// COLLECTIONS

	collectionsGroup := apiGroupV1.Group("/collections")

	collectionsGroup.GET("/get", jwtOptional(), getCollectionByID(repos.collectionRepository, repos.tokenRepository, ipfsClient))
	collectionsGroup.GET("/user_get", jwtOptional(), getCollectionsByUserID(repos.collectionRepository, repos.tokenRepository, ipfsClient))
	collectionsGroup.POST("/create", jwtRequired(), createCollection(repos.collectionRepository, repos.galleryRepository))
	collectionsGroup.POST("/delete", jwtRequired(), deleteCollection(repos.collectionRepository))
	collectionsGroup.POST("/update/info", jwtRequired(), updateCollectionInfo(repos.collectionRepository))
	collectionsGroup.POST("/update/hidden", jwtRequired(), updateCollectionHidden(repos.collectionRepository))
	collectionsGroup.POST("/update/nfts", jwtRequired(), updateCollectionNfts(repos.collectionRepository))

	// NFTS

	nftsGroup := apiGroupV1.Group("/nfts")

	nftsGroup.GET("/get", jwtOptional(), getTokenByID(repos.tokenRepository, ipfsClient))
	nftsGroup.GET("/user_get", jwtOptional(), getTokensForUser(repos.tokenRepository, ipfsClient))
	nftsGroup.POST("/update", jwtRequired(), updateTokenByID(repos.tokenRepository))
	nftsGroup.GET("/get_unassigned", jwtRequired(), getUnassignedTokensForUser(repos.collectionRepository, repos.tokenRepository, ipfsClient))

	// HEALTH
	apiGroupV1.GET("/health", healthcheck())

	return router
}

func authHandlersInit(parent *gin.RouterGroup, repos *repositories, ethClient *eth.Client) {

	usersGroup := parent.Group("/users")

	authGroup := parent.Group("/auth")

	// AUTH_GET_PREFLIGHT
	// UN-AUTHENTICATED

	// called before login/sugnup calls, mostly to get nonce and also discover if user exists.

	// [GET] /glry/v1/auth/get_preflight?address=:walletAddress
	authGroup.GET("/get_preflight", jwtOptional(), getAuthPreflight(repos.userRepository, repos.nonceRepository, ethClient))

	// AUTH VALIDATE_JWT

	// [GET] /glry/v1/auth/jwt_valid
	authGroup.GET("/jwt_valid", jwtOptional(), validateJwt())

	// AUTH_USER_LOGIN
	// UN-AUTHENTICATED

	usersGroup.POST("/login", login(repos.userRepository, repos.nonceRepository, repos.loginRepository))

	// USER_UPDATE
	// AUTHENTICATED

	usersGroup.POST("/update/info", jwtRequired(), updateUserInfo(repos.userRepository, ethClient))
	usersGroup.POST("/update/addresses/add", jwtRequired(), addUserAddress(repos.userRepository, repos.nonceRepository))
	usersGroup.POST("/update/addresses/remove", jwtRequired(), removeAddresses(repos.userRepository, repos.collectionRepository))

	// USER_GET
	// AUTHENTICATED/UN-AUTHENTICATED

	usersGroup.GET("/get", jwtOptional(), getUser(repos.userRepository))

	// USER_CREATE
	// UN-AUTHENTICATED

	usersGroup.POST("/create", createUser(repos.userRepository, repos.nonceRepository, repos.galleryRepository))

}
