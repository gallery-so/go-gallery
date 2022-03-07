package feedbot

import (
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/persist"
)

func handlersInit(router *gin.Engine, userRepo persist.UserRepository, userEventRepo persist.UserEventRepository, tokenEventRepo persist.TokenEventRepository, collectionEventRepo persist.CollectionEventRepository) *gin.Engine {
	router.GET("/ping", ping())
	router.POST("/tasks/feed-events", middleware.TaskRequired(), handleMessage(userRepo, userEventRepo, tokenEventRepo, collectionEventRepo))
	return router
}
