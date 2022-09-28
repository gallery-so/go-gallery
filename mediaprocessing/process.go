package mediaprocessing

import (
	"context"
	"net/http"
	"sort"
	"time"

	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	"github.com/gammazero/workerpool"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/util"
)

type ProcessMediaInput struct {
	UserID            persist.DBID  `json:"user_id" binding:"required"`
	Chain             persist.Chain `json:"chain" binding:"required"`
	ImageKeywords     []string      `json:"image_keywords" binding:"required"`
	AnimationKeywords []string      `json:"animation_keywords" binding:"required"`
}

func processMediaForUsersTokensOfChain(queue chan<- ProcessMediaInput, tokenRepo persist.TokenGalleryRepository, contractRepo persist.ContractGalleryRepository, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, throttler *throttle.Locker) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input ProcessMediaInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		if err := throttler.Lock(c, input.UserID.String()); err != nil {
			util.ErrResponse(c, http.StatusTooManyRequests, err)
			return
		}

		allTokens, err := tokenRepo.GetByUserID(c, input.UserID, -1, -1)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		filtered := make([]persist.TokenGallery, 0, len(allTokens))
		for _, token := range allTokens {
			if token.Chain == input.Chain {
				filtered = append(filtered, token)
			}
		}
		sort.Slice(allTokens, func(i, j int) bool {
			if allTokens[i].Media.IsServable() && !allTokens[j].Media.IsServable() {
				return false
			}
			return true
		})

		innerWp := workerpool.New(100)
		for _, token := range allTokens {
			t := token
			innerWp.Submit(func() {
				totalTimeOfWp := time.Now()
				ctx, cancel := context.WithTimeout(c, time.Minute*10)
				defer cancel()

				logger.For(ctx).Infof("Processing Media: %s - Processing Token: %s-%s-%d", input.UserID, t.Contract, t.TokenID, input.Chain)
				image, animation := media.KeywordsForChain(input.Chain, input.ImageKeywords, input.AnimationKeywords)

				contract, err := contractRepo.GetByID(ctx, t.Contract)
				if err != nil {
					logger.For(ctx).Errorf("Error getting contract: %s", err)
					return
				}
				totalTimeOfMedia := time.Now()
				med, err := media.MakePreviewsForMetadata(ctx, t.TokenMetadata, contract.Address, persist.TokenID(t.TokenID.String()), t.TokenURI, input.Chain, ipfsClient, arweaveClient, stg, tokenBucket, image, animation)
				if err != nil {
					logger.For(ctx).Errorf("error processing media for %s: %s", input.UserID, err)
					med = persist.Media{
						MediaType: persist.MediaTypeUnknown,
					}
				}
				logger.For(ctx).Infof("Processing Media: %s - Processing Token: %s-%s-%d - Took: %s", input.UserID, contract.Address, t.TokenID, input.Chain, time.Since(totalTimeOfMedia))

				up := persist.TokenUpdateMediaInput{
					Media:       med,
					Metadata:    t.TokenMetadata,
					TokenURI:    t.TokenURI,
					Name:        persist.NullString(t.Name),
					Description: persist.NullString(t.Description),
					LastUpdated: persist.LastUpdatedTime{},
				}
				totalUpdateTime := time.Now()
				logger.For(ctx).Infof("Processing Media: %s - Processing Token: %s-%s-%d - Updating Token", input.UserID, contract.Address, t.TokenID, input.Chain)
				if err := tokenRepo.UpdateByTokenIdentifiersUnsafe(ctx, t.TokenID, contract.Address, input.Chain, up); err != nil {
					logger.For(ctx).Errorf("error updating media for %s-%s-%d: %s", t.TokenID, contract.Address, input.Chain, err)
					return
				}
				logger.For(ctx).Infof("Processing Media: %s - Processing Token: %s-%s-%d - Update Took: %s", input.UserID, contract.Address, t.TokenID, input.Chain, time.Since(totalUpdateTime))

				logger.For(ctx).Infof("Processing Media: %s - Finished Processing Token: %s-%s-%d | Took %s", input.UserID, contract.Address, t.TokenID, input.Chain, time.Since(totalTimeOfWp))
			})
		}
		func() {
			defer throttler.Unlock(c, input.UserID.String())
			innerWp.StopWait()
			logger.For(nil).Infof("Processing Media: %s - Finished", input.UserID)
		}()
		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}
