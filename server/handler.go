package server

import (
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/runtime"
)

//-------------------------------------------------------------
func HandlersInit(pRuntime *runtime.Runtime) *gin.Engine {

	apiGroupV1 := pRuntime.Router.Group("/glry/v1")

	// AUTH_HANDLERS
	// TODO: bring these handlers out to this file and format
	// like the routes below
	authHandlersInit(pRuntime, apiGroupV1)

	//-------------------------------------------------------------
	// COLLECTIONS
	//-------------------------------------------------------------

	collectionsGroup := apiGroupV1.Group("/collections")

	collectionsGroup.GET("/get", jwtOptional(pRuntime), getAllCollectionsForUser(pRuntime))
	collectionsGroup.POST("/create", jwtRequired(pRuntime), createCollection(pRuntime))
	collectionsGroup.POST("/delete", jwtRequired(pRuntime), deleteCollection(pRuntime))

	//-------------------------------------------------------------
	// NFTS
	//-------------------------------------------------------------

	nftsGroup := apiGroupV1.Group("/nfts")

	nftsGroup.GET("/get", jwtOptional(pRuntime), getNftById(pRuntime))
	nftsGroup.GET("/user_get", jwtOptional(pRuntime), getNftsForUser(pRuntime))
	nftsGroup.GET("/opensea_get", jwtOptional(pRuntime), getNftsFromOpensea(pRuntime))
	nftsGroup.POST("/update", jwtRequired(pRuntime), updateNftById(pRuntime))

	// HEALTH
	apiGroupV1.GET("/health", healthcheck(pRuntime))

	//-------------------------------------------------------------

	return pRuntime.Router
}

func authHandlersInit(pRuntime *runtime.Runtime, parent *gin.RouterGroup) {

	usersGroup := parent.Group("/users")
	usersGroup.Use(jwtRequired(pRuntime))

	authGroup := parent.Group("/auth")

	//-------------------------------------------------------------
	// AUTH_GET_PREFLIGHT
	// UN-AUTHENTICATED

	// called before login/sugnup calls, mostly to get nonce and also discover if user exists.

	// [GET] /glry/v1/auth/get_preflight?addr=:walletAddress
	authGroup.GET("/get_preflight", jwtOptional(pRuntime), getAuthPreflight(pRuntime))

	//-------------------------------------------------------------
	// AUTH_USER_LOGIN
	// UN-AUTHENTICATED

	usersGroup.POST("/login", login(pRuntime))

	//-------------------------------------------------------------
	// USER_UPDATE
	// AUTHENTICATED

	usersGroup.POST("/update", updateUserAuth(pRuntime))

	//-------------------------------------------------------------
	// USER_GET
	// AUTHENTICATED/UN-AUTHENTICATED

	usersGroup.GET("/get", getUserAuth(pRuntime))

	//-------------------------------------------------------------
	// USER_CREATE
	// UN-AUTHENTICATED

	usersGroup.POST("/create", createUserAuth(pRuntime))

}
