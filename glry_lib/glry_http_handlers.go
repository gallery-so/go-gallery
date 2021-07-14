package glry_lib

import (
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/glry_core"
)

//-------------------------------------------------------------
func HandlersInit(pRuntime *glry_core.Runtime) *gin.Engine {

	apiGroupV1 := pRuntime.Router.Group("/glry/v1")

	// AUTH_HANDLERS
	// TODO: bring these handlers out to this file and format
	// like the routes below
	AuthHandlersInit(pRuntime, apiGroupV1)

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

	// HEALTH
	apiGroupV1.GET("/health", healthcheck(pRuntime))

	//-------------------------------------------------------------

	return pRuntime.Router
}
