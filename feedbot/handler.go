package feedbot

import (
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/middleware"
)

func handlersInit(router *gin.Engine, eventRepos *event.EventRepositories) *gin.Engine {
	router.GET("/ping", ping())
	router.POST("/tasks/feed-events", middleware.TaskRequired(), handleMessage(eventRepos))
	return router
}
