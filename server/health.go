package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/runtime"
)

type HealthcheckResponse struct {
	Message string `json:"msg"`
	Env     string `json:"env"`
}

func healthcheck(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, HealthcheckResponse{
			Message: "gallery operational",
			Env:     pRuntime.Config.EnvStr,
		})
	}
}
