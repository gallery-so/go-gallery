package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

func proxySnapshot() gin.HandlerFunc {
	return func(c *gin.Context) {
		jsn, err := http.Get(viper.GetString("SNAPSHOT_LINK"))
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.DataFromReader(http.StatusOK, jsn.ContentLength, "application/json", jsn.Body, nil)
	}
}
