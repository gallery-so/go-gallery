package features

import (
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
)

func handlersInit(router *gin.Engine, featuresRepository persist.FeatureFlagRepository, accessRepository persist.AccessRepository) *gin.Engine {

	return router
}
