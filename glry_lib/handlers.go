package glry_lib

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/glry_core"
)

type HealthcheckResponse struct {
	Message string `json:"msg"`
	Env 	string `json:"env"`
}

func healthcheck(pRuntime *glry_core.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, HealthcheckResponse{
			Message: "gallery operational",
			Env: pRuntime.Config.EnvStr,
		})
	}
}