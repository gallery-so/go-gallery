package server

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/runtime"
)

type healthcheckResponse struct {
	Message string `json:"msg"`
	Env     string `json:"env"`
}

func healthcheck(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, healthcheckResponse{
			Message: "gallery operational",
			Env:     pRuntime.Config.EnvStr,
		})
	}
}

func nuke(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		err := pRuntime.DB.MongoClient.Database(runtime.GalleryDBName).Drop(context.Background())
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{
				Error: err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, successOutput{
			Success: true,
		})
	}
}
