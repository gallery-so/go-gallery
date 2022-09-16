package mediaprocessing

import (
	"context"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	"github.com/gammazero/workerpool"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/util"
)

type ProcessMediaInput struct {
	Key               string                          `json:"key" binding:"required"`
	Chain             persist.Chain                   `json:"chain" binding:"required"`
	Tokens            []multichain.ChainAgnosticToken `json:"tokens" binding:"required"`
	ImageKeywords     []string                        `json:"image_keywords" binding:"required"`
	AnimationKeywords []string                        `json:"animation_keywords" binding:"required"`
}

func processIPFSMetadata(queue chan<- ProcessMediaInput, throttler *throttle.Locker) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input ProcessMediaInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		if err := throttler.Lock(c, input.Key); err != nil {
			util.ErrResponse(c, http.StatusTooManyRequests, err)
			return
		}
		queue <- input
		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processMedias(queue <-chan ProcessMediaInput, tokenRepo persist.TokenGalleryRepository, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, throttler *throttle.Locker) {
	wp := workerpool.New(10)
	for processInput := range queue {
		in := processInput
		logger.For(nil).Infof("Processing Media: %s", in.Key)
		wp.Submit(func() {
			innerWp := workerpool.New(10)
			for _, token := range in.Tokens {
				t := token
				innerWp.Submit(func() {
					ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
					defer cancel()

					logger.For(ctx).Infof("Processing Media: %s - Processing Token: %s-%s-%s", in.Key, t.ContractAddress, t.TokenID, in.Chain)
					image, animation := media.KeywordsForChain(in.Chain, in.ImageKeywords, in.AnimationKeywords)

					media, err := media.MakePreviewsForMetadata(ctx, t.TokenMetadata, t.ContractAddress, persist.TokenID(t.TokenID.String()), t.TokenURI, in.Chain, ipfsClient, arweaveClient, stg, tokenBucket, image, animation)
					if err != nil {
						logger.For(ctx).Errorf("error processing media for %s: %v", in.Key, err)
						return
					}
					up := persist.TokenUpdateMediaInput{
						Media:       media,
						Metadata:    t.TokenMetadata,
						TokenURI:    t.TokenURI,
						Name:        persist.NullString(t.Name),
						Description: persist.NullString(t.Description),
						LastUpdated: persist.LastUpdatedTime{},
					}
					if err := tokenRepo.UpdateByTokenIdentifiersUnsafe(ctx, t.TokenID, t.ContractAddress, in.Chain, up); err != nil {
						logger.For(ctx).Errorf("error updating media for %s-%s-%s: %s %v", t.TokenID, t.ContractAddress, in.Chain, err)
						return
					}
					logger.For(ctx).Infof("Processing Media: %s - Finished Processing Token: %s-%s-%s", in.Key, t.ContractAddress, t.TokenID, in.Chain)
				})
			}
			func() {
				defer throttler.Unlock(context.Background(), in.Key)
				innerWp.StopWait()
				logger.For(nil).Infof("Processing Media: %s - Finished", in.Key)
			}()
		})
	}
}
