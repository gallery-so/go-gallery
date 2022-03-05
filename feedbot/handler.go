package feedbot

import "github.com/gin-gonic/gin"

func handlersInit(router *gin.Engine) *gin.Engine {
	router.GET("/ping", ping())
	return router
}
