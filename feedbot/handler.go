package feedbot

import (
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
)

func handlersInit(router *gin.Engine, eventRepo *postgres.EventRepository) *gin.Engine {
	router.GET("/ping", ping())
	router.POST("/tasks/feed-events", middleware.TaskRequired(), handleEvent(eventRepo, eventRoutes()))
	return router
}

func eventRoutes() EventToRoute {
	return map[persist.EventType]func(*gin.Context, *postgres.EventRepository, Event){
		eventTypeUserCreated:          handleEventNewUser,
		eventTypeUpdateNFT:            handleEventUpdateNFT,
		eventTypeNewCollection:        handleEventNewCollection,
		eventTypeUpdateCollectionInfo: handleEventUpdateCollectionInfo,
		eventTypeUpdateCollectionNFTs: handleEventUpdateCollectionNFTs,
	}
}
