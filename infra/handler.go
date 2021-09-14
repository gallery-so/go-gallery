package infra

import (
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/runtime"
)

func handlersInit(pRuntime *runtime.Runtime) *gin.Engine {

	apiGroupV1 := pRuntime.Router.Group("/infra/v1")

	tokensGroup := apiGroupV1.Group("/tokens")

	tokensGroup.GET("/get", getERC721Tokens(pRuntime))

	return pRuntime.Router
}
