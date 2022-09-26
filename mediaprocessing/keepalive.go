package mediaprocessing

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

func keepAlive() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"success": true})
	}
}

func pingKeepAlive(ctx context.Context) error {
	url := fmt.Sprintf("%s/keepalive", viper.GetString("SELF_HOST"))
	logger.For(ctx).Infof("pinging keepalive at %s", url)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return util.GetErrFromResp(resp)
	}
	return nil
}

func keepAliveUntilDone(done chan struct{}, taskName string) {
	for {
		select {
		case <-done:
			return
		case <-time.After(30 * time.Second):
			err := func() error {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				return pingKeepAlive(ctx)
			}()
			if err != nil {
				logger.For(nil).Errorf("error pinging keepalive at %s/keepalive: %s", viper.GetString("SELF_HOST"), err)
			}
			logger.For(nil).Infof("keep-alive during: %s", taskName)
		}
	}
}
