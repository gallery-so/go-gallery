package server

import (
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/runtime"
)

func handlersInit(pRuntime *runtime.Runtime) *gin.Engine {

	apiGroupV1 := pRuntime.Router.Group("/glry/v1")

	// AUTH_HANDLERS
	authHandlersInit(pRuntime, apiGroupV1)

	// GALLERIES

	galleriesGroup := apiGroupV1.Group("/galleries")

	galleriesGroup.GET("/get", jwtOptional(pRuntime), getGalleryByID(pRuntime))
	galleriesGroup.GET("/user_get", jwtOptional(pRuntime), getGalleriesByUserID(pRuntime))
	galleriesGroup.POST("/update", jwtRequired(pRuntime), updateGallery(pRuntime))

	// COLLECTIONS

	collectionsGroup := apiGroupV1.Group("/collections")

	collectionsGroup.GET("/get", jwtOptional(pRuntime), getCollectionByID(pRuntime))
	collectionsGroup.GET("/user_get", jwtOptional(pRuntime), getCollectionsByUserID(pRuntime))
	collectionsGroup.POST("/create", jwtRequired(pRuntime), createCollection(pRuntime))
	collectionsGroup.POST("/delete", jwtRequired(pRuntime), deleteCollection(pRuntime))
	collectionsGroup.POST("/update/info", jwtRequired(pRuntime), updateCollectionInfo(pRuntime))
	collectionsGroup.POST("/update/hidden", jwtRequired(pRuntime), updateCollectionHidden(pRuntime))
	collectionsGroup.POST("/update/nfts", jwtRequired(pRuntime), updateCollectionNfts(pRuntime))

	// NFTS

	nftsGroup := apiGroupV1.Group("/nfts")

	nftsGroup.GET("/get", jwtOptional(pRuntime), getNftByID(pRuntime))
	nftsGroup.GET("/user_get", jwtOptional(pRuntime), getNftsForUser(pRuntime))
	nftsGroup.GET("/sync", rateLimited(pRuntime), jwtRequired(pRuntime), syncNftsFromBlockChain(pRuntime))
	nftsGroup.POST("/update", jwtRequired(pRuntime), updateNftByID(pRuntime))
	nftsGroup.GET("/get_unassigned", jwtRequired(pRuntime), getUnassignedNftsForUser(pRuntime))

	// HEALTH
	apiGroupV1.GET("/health", healthcheck(pRuntime))
	if pRuntime.Config.Env == "development" || pRuntime.Config.Env == "local" {
		apiGroupV1.GET("/nuke", nuke(pRuntime))
	}

	return pRuntime.Router
}

func authHandlersInit(pRuntime *runtime.Runtime, parent *gin.RouterGroup) {

	usersGroup := parent.Group("/users")

	authGroup := parent.Group("/auth")

	// AUTH_GET_PREFLIGHT
	// UN-AUTHENTICATED

	// called before login/sugnup calls, mostly to get nonce and also discover if user exists.

	// [GET] /glry/v1/auth/get_preflight?address=:walletAddress
	authGroup.GET("/get_preflight", jwtOptional(pRuntime), getAuthPreflight(pRuntime))

	// AUTH VALIDATE_JWT

	// [GET] /glry/v1/auth/jwt_valid
	authGroup.GET("/jwt_valid", jwtOptional(pRuntime), validateJwt(pRuntime))

	// AUTH_USER_LOGIN
	// UN-AUTHENTICATED

	usersGroup.POST("/login", login(pRuntime))

	// USER_UPDATE
	// AUTHENTICATED

	usersGroup.POST("/update/info", jwtRequired(pRuntime), updateUserInfo(pRuntime))
	usersGroup.POST("/update/addresses/add", jwtRequired(pRuntime), addUserAddress(pRuntime))
	usersGroup.POST("/update/addresses/remove", jwtRequired(pRuntime), removeAddresses(pRuntime))

	// USER_GET
	// AUTHENTICATED/UN-AUTHENTICATED

	usersGroup.GET("/get", jwtOptional(pRuntime), getUser(pRuntime))

	// USER_CREATE
	// UN-AUTHENTICATED

	usersGroup.POST("/create", createUser(pRuntime))

}
