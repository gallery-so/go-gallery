package server

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/mikeydub/go-gallery/util"
)

type healthcheckResponse struct {
	Message string `json:"msg"`
	Env     string `json:"env"`
}

func healthcheck(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, healthcheckResponse{
			Message: "gallery operational",
			Env:     pRuntime.Config.Env,
		})
	}
}

func nuke(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		err := pRuntime.DB.MongoClient.Database(runtime.GalleryDBName).Drop(context.Background())
		if err != nil {
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{
				Error: err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, util.SuccessResponse{
			Success: true,
		})
	}
}
