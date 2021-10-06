package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type healthcheckResponse struct {
	Message string `json:"msg"`
	Env     string `json:"env"`
}

func healthcheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, healthcheckResponse{
			Message: "gallery operational",
			Env:     env,
		})
	}
}
