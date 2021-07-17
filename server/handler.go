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
	apiGroupV1.GET("/collections/get", getAllCollectionsForUser(pRuntime))
	apiGroupV1.POST("/collections/create", createCollection(pRuntime))
	apiGroupV1.POST("/collections/delete", deleteCollection(pRuntime))

	//-------------------------------------------------------------
	// NFTS
	//-------------------------------------------------------------
	apiGroupV1.GET("/nfts/get", getNftById(pRuntime))
	apiGroupV1.GET("/nfts/user_get", getNftsForUser(pRuntime))
	apiGroupV1.GET("/nfts/opensea_get", getNftsFromOpensea(pRuntime))
	apiGroupV1.POST("/nfts/update", updateNftById(pRuntime))
	apiGroupV1.GET("/nfts/get_unassigned", getUnassignedNftsForUser(pRuntime))

	// HEALTH
	apiGroupV1.GET("/health", healthcheck(pRuntime))

	//-------------------------------------------------------------

	return pRuntime.Router
}

func authHandlersInit(pRuntime *runtime.Runtime, parent *gin.RouterGroup) {

	usersGroup := parent.Group("/users")
	usersGroup.Use(jwtMiddleware(pRuntime))

	//-------------------------------------------------------------
	// AUTH_GET_PREFLIGHT
	// UN-AUTHENTICATED

	// called before login/sugnup calls, mostly to get nonce and also discover if user exists.

	// [GET] /glry/v1/auth/get_preflight?addr=:walletAddress
	usersGroup.GET("/auth/get_preflight", getAuthPreflight(pRuntime))

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
