package feedbot

import (
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
)

func handlersInit(router *gin.Engine, eventRepo *postgres.EventRepository) *gin.Engine {
	feedTasks := router.Group("/tasks/feed-events")
	router.GET("/ping", ping())
	feedTasks.POST("/users", middleware.TaskRequired(), eventNewUser(eventRepo))
	feedTasks.POST("/nfts/update", middleware.TaskRequired(), eventUpdateNFT(eventRepo))
	feedTasks.POST("/collections", middleware.TaskRequired(), eventNewCollection(eventRepo))
	feedTasks.POST("/collections/update/info", middleware.TaskRequired(), eventUpdateCollectionInfo(eventRepo))
	feedTasks.POST("/collections/update/nfts", middleware.TaskRequired(), eventUpdateCollectionNFTs(eventRepo))
	return router
}
