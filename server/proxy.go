package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/util"
)

func proxySnapshot() gin.HandlerFunc {
	return func(c *gin.Context) {
		jsn, err := http.Get("https://storage.googleapis.com/gallery-dev-322005.appspot.com/snapshot.json")
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.DataFromReader(http.StatusOK, jsn.ContentLength, "application/json", jsn.Body, nil)
	}
}
