package server

import (
	"github.com/mikeydub/go-gallery/service/logger"
	"net/http"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

func proxySnapshot(stg *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		snpBucket := viper.GetString("SNAPSHOT_BUCKET")
		logger.For(c).Infof("Proxying snapshot from bucket %s", snpBucket)

		obj := stg.Bucket(viper.GetString("SNAPSHOT_BUCKET")).Object("snapshot.json")
		r, err := obj.NewReader(c)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		defer r.Close()
		c.DataFromReader(http.StatusOK, int64(r.Size()), "application/json", r, nil)
	}
}
