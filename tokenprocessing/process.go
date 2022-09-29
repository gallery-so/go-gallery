package tokenprocessing

import (
	"context"
	"fmt"
	"net/http"
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

type ProcessMediaForUserInput struct {
	UserID            persist.DBID  `json:"user_id" binding:"required"`
	Chain             persist.Chain `json:"chain" binding:"required"`
	ImageKeywords     []string      `json:"image_keywords" binding:"required"`
	AnimationKeywords []string      `json:"animation_keywords" binding:"required"`
}

type ProcessMediaForTokenInput struct {
	TokenID           persist.TokenID `json:"token_id" binding:"required"`
	ContractAddress   persist.Address `json:"contract_address" binding:"required"`
	Chain             persist.Chain   `json:"chain" binding:"required"`
	OwnerAddress      persist.Address `json:"owner_address" binding:"required"`
	ImageKeywords     []string        `json:"image_keywords" binding:"required"`
	AnimationKeywords []string        `json:"animation_keywords" binding:"required"`
}

func processMediaForUsersTokensOfChain(tokenRepo persist.TokenGalleryRepository, contractRepo persist.ContractGalleryRepository, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, throttler *throttle.Locker) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input ProcessMediaForUserInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}
		if err := throttler.Lock(c, input.UserID.String()); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		allTokens, err := tokenRepo.GetByUserID(c, input.UserID, -1, -1)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		filtered := make([]persist.TokenGallery, 0, len(allTokens))
		for _, token := range allTokens {
			if token.Chain == input.Chain && !token.Media.IsServable() {
				filtered = append(filtered, token)
			}
		}

		innerWp := workerpool.New(100)
		for _, token := range filtered {
			t := token
			contract, err := contractRepo.GetByID(c, t.Contract)
			if err != nil {
				logger.For(c).Errorf("Error getting contract: %s", err)
			}
			innerWp.Submit(func() {
				key := fmt.Sprintf("%s-%s-%d", t.TokenID, contract.Address, t.Chain)
				err := processToken(c, key, t, contract.Address, ipfsClient, arweaveClient, stg, tokenBucket, tokenRepo, input.ImageKeywords, input.AnimationKeywords)
				if err != nil {
					logger.For(c).Errorf("Error processing token: %s", err)
				}
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

func processMediaForToken(tokenRepo persist.TokenGalleryRepository, userRepo persist.UserRepository, walletRepo persist.WalletRepository, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, throttler *throttle.Locker) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input ProcessMediaForTokenInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		key := fmt.Sprintf("%s-%s-%d", input.TokenID, input.ContractAddress, input.Chain)
		if err := throttler.Lock(c, key); err != nil {
			util.ErrResponse(c, http.StatusTooManyRequests, err)
			return
		}
		defer throttler.Unlock(c, key)

		wallet, err := walletRepo.GetByChainAddress(c, persist.NewChainAddress(input.OwnerAddress, input.Chain))
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		user, err := userRepo.GetByWalletID(c, wallet.ID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		t, err := tokenRepo.GetByFullIdentifiers(c, input.TokenID, input.ContractAddress, input.Chain, user.ID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		err = processToken(c, key, t, input.ContractAddress, ipfsClient, arweaveClient, stg, tokenBucket, tokenRepo, input.ImageKeywords, input.AnimationKeywords)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processToken(c context.Context, key string, t persist.TokenGallery, contractAddress persist.Address, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, tokenRepo persist.TokenGalleryRepository, imageKeywords, animationKeywords []string) error {
	totalTime := time.Now()
	ctx, cancel := context.WithTimeout(c, time.Minute*10)
	defer cancel()

	logger.For(ctx).Infof("Processing Media: %s - Processing Token: %s-%s-%d", key, contractAddress, t.TokenID, t.Chain)
	image, animation := media.KeywordsForChain(t.Chain, imageKeywords, animationKeywords)

	totalTimeOfMedia := time.Now()
	med, err := media.MakePreviewsForMetadata(ctx, t.TokenMetadata, contractAddress, persist.TokenID(t.TokenID.String()), t.TokenURI, t.Chain, ipfsClient, arweaveClient, stg, tokenBucket, image, animation)
	if err != nil {
		logger.For(ctx).Errorf("error processing media for %s: %s", key, err)
		med = persist.Media{
			MediaType: persist.MediaTypeUnknown,
		}
	}
	logger.For(ctx).Infof("Processing Media: %s - Processing Token: %s-%s-%d - Took: %s", key, contractAddress, t.TokenID, t.Chain, time.Since(totalTimeOfMedia))

	up := persist.TokenUpdateMediaInput{
		Media:       med,
		Metadata:    t.TokenMetadata,
		TokenURI:    t.TokenURI,
		Name:        persist.NullString(t.Name),
		Description: persist.NullString(t.Description),
		LastUpdated: persist.LastUpdatedTime{},
	}
	totalUpdateTime := time.Now()
	logger.For(ctx).Infof("Processing Media: %s - Processing Token: %s-%s-%d - Updating Token", key, contractAddress, t.TokenID, t.Chain)
	if err := tokenRepo.UpdateByTokenIdentifiersUnsafe(ctx, t.TokenID, contractAddress, t.Chain, up); err != nil {
		logger.For(ctx).Errorf("error updating media for %s-%s-%d: %s", t.TokenID, contractAddress, t.Chain, err)
		return err
	}

	logger.For(ctx).Infof("Processing Media: %s - Processing Token: %s-%s-%d - Update Took: %s", key, contractAddress, t.TokenID, t.Chain, time.Since(totalUpdateTime))

	logger.For(ctx).Infof("Processing Media: %s - Finished Processing Token: %s-%s-%d | Took %s", key, contractAddress, t.TokenID, t.Chain, time.Since(totalTime))
	return nil
}

// func processTokensInCollectionRefreshes(queue <-chan ProcessCollectionTokensRefreshInput, mc *multichain.Provider, throttler *throttle.Locker) {
// 	wp := workerpool.New(10)
// 	for processInput := range queue {
// 		in := processInput
// 		logger.For(nil).Infof("Processing Collection Tokens Refresh: %s", in.Key)
// 		wp.Submit(func() {
// 			ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
// 			defer cancel()
// 			if err := throttler.Lock(ctx, in.Key); err != nil {
// 				logger.For(ctx).Errorf("error locking: %v", err)
// 				return
// 			}
// 			defer throttler.Unlock(ctx, in.Key)
// 			logger.For(ctx).Infof("Processing: %s - Processing Collection Refresh", in.Key)
// 			if err := mc.RefreshTokensForCollection(ctx, in.ContractIdentifiers); err != nil {
// 				logger.For(ctx).Errorf("error processing media for %s: %v", in.Key, err)
// 				return
// 			}
// 			logger.For(ctx).Infof("Processing: %s - Finished Processing Collection Refresh", in.Key)

// 		})
// 	}
// }
