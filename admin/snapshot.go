package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	storage "cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var rwMutex = &sync.RWMutex{}

type snapshot struct {
	Snapshot []string `json:"snapshot" binding:"required"`
}

func getSnapshot(stg *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		rwMutex.RLock()
		defer rwMutex.RUnlock()
		r, err := getSnapshotReader(c, stg)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		defer r.Close()
		c.DataFromReader(http.StatusOK, int64(r.Size()), "application/json", r, nil)
	}
}

func updateSnapshot(stg *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input snapshot
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		logrus.Infof("updating snapshot: %d", len(input.Snapshot))
		rwMutex.Lock()
		defer rwMutex.Unlock()

		err := writeSnapshot(c, stg, input.Snapshot)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func getSnapshotReader(c context.Context, stg *storage.Client) (*storage.Reader, error) {
	r, err := stg.Bucket(viper.GetString("SNAPSHOT_BUCKET")).Object("snapshot.json").NewReader(c)
	if err != nil {
		return nil, err
	}
	return r, nil
}
func writeSnapshot(c context.Context, stg *storage.Client, snapshot []string) error {
	obj := stg.Bucket(viper.GetString("SNAPSHOT_BUCKET")).Object("snapshot.json")
	obj.Delete(c)
	w := obj.NewWriter(c)
	w.CacheControl = "no-store"
	w.ContentType = "application/json"
	w.Metadata = map[string]string{
		"Cache-Control": "no-store",
	}

	err := json.NewEncoder(w).Encode(snapshot)
	if err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}

	return obj.ACL().Set(c, storage.AllUsers, storage.RoleReader)
}
