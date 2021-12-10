package features

import (
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/persist"
)

func handlersInit(router *gin.Engine, userRepo persist.UserRepository, featuresRepository persist.FeatureFlagRepository, accessRepository persist.AccessRepository, ethClient *ethclient.Client) *gin.Engine {
	parent := router.Group("/features/v1")
	access := parent.Group("/access")
	access.GET("/user_get", getUserFeatures(userRepo, featuresRepository, accessRepository, ethClient))
	return router
}
