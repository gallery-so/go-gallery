package tokenprocessing

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

type ProcessMediaForTokenInput struct {
	TokenID           persist.TokenID `json:"token_id" binding:"required"`
	ContractAddress   persist.Address `json:"contract_address" binding:"required"`
	Chain             persist.Chain   `json:"chain"`
	OwnerAddress      persist.Address `json:"owner_address" binding:"required"`
	ImageKeywords     []string        `json:"image_keywords" binding:"required"`
	AnimationKeywords []string        `json:"animation_keywords" binding:"required"`
}

func processMediaForUsersTokensOfChain(tokenRepo persist.TokenGalleryRepository, contractRepo persist.ContractGalleryRepository, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, throttler *throttle.Locker) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input task.TokenProcessingUserMessage
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		ctx := logger.NewContextWithFields(c, logrus.Fields{"userID": input.UserID})
		// XXX: span, ctx := tracing.StartSpan(ctx, "processMediaForUsersTokensOfChain", fmt.Sprintf("userID=%s;chain=%d", input.UserID, input.Chain))
		// XXX: defer tracing.FinishSpan(span)

		// XXX: if err := throttler.Lock(ctx, input.UserID.String()); err != nil {
		// XXX: 	util.ErrResponse(c, http.StatusOK, err)
		// XXX: 	return
		// XXX: }

		allTokens, err := tokenRepo.GetByUserID(ctx, input.UserID, -1, -1)
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

		var i int
		for _, token := range filtered {
			i++
			if i > 5 {
				break
			}
			t := token
			contract, err := contractRepo.GetByID(ctx, t.Contract)
			if err != nil {
				logger.For(ctx).WithError(err).Error("error getting contract")
			}
			key := fmt.Sprintf("%s-%s-%d", t.TokenID, contract.Address, t.Chain)
			err = processToken(ctx, key, t, contract.Address, ipfsClient, arweaveClient, stg, tokenBucket, tokenRepo, input.ImageKeywords, input.AnimationKeywords)
			if err != nil {
				logger.For(ctx).WithError(err).Error("error processing token")
			}
		}

		// XXX: wp := workerpool.New(100)
		// XXX: wp := workerpool.New(13)
		// XXX: for _, token := range filtered {
		// XXX: 	t := token
		// XXX: 	contract, err := contractRepo.GetByID(ctx, t.Contract)
		// XXX: 	if err != nil {
		// XXX: 		logger.For(ctx).WithError(err).Error("error getting contract")
		// XXX: 	}
		// XXX: 	wp.Submit(func() {
		// XXX: 		key := fmt.Sprintf("%s-%s-%d", t.TokenID, contract.Address, t.Chain)
		// XXX: 		ctx := sentryutil.NewSentryHubContext(c)
		// XXX: 		err := processToken(ctx, key, t, contract.Address, ipfsClient, arweaveClient, stg, tokenBucket, tokenRepo, input.ImageKeywords, input.AnimationKeywords)
		// XXX: 		if err != nil {
		// XXX: 			logger.For(ctx).WithError(err).Error("error processing token")
		// XXX: 		}
		// XXX: 	})
		// XXX: }
		// XXX:
		// XXX: wp.StopWait()
		logger.For(ctx).Info("processing media finished")

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
	ctx := logger.NewContextWithFields(c, logrus.Fields{
		"tokenID":         t.TokenID,
		"contractAddress": contractAddress,
		"chain":           t.Chain,
	})
	totalTime := time.Now()
	ctx, cancel := context.WithTimeout(ctx, time.Minute*10)
	defer cancel()

	logger.For(ctx).Infof("processing media")
	image, animation := media.KeywordsForChain(t.Chain, imageKeywords, animationKeywords)

	name, description := media.FindNameAndDescription(ctx, t.TokenMetadata)

	if name == "" {
		name = t.Name.String()
	}

	if description == "" {
		description = t.Description.String()
	}

	totalTimeOfMedia := time.Now()
	med, err := media.MakePreviewsForMetadata(ctx, t.TokenMetadata, contractAddress, persist.TokenID(t.TokenID.String()), t.TokenURI, t.Chain, ipfsClient, arweaveClient, stg, tokenBucket, image, animation)
	if err != nil {
		logger.For(ctx).WithError(err).Error("error processing media")
		med = persist.Media{
			MediaType: persist.MediaTypeUnknown,
		}
	}
	logger.For(ctx).Infof("processing media took %s", time.Since(totalTimeOfMedia))

	up := persist.TokenUpdateAllURIDerivedFieldsInput{
		Media:       med,
		Metadata:    t.TokenMetadata,
		TokenURI:    t.TokenURI,
		Name:        persist.NullString(name),
		Description: persist.NullString(description),
		LastUpdated: persist.LastUpdatedTime{},
	}
	totalUpdateTime := time.Now()
	logger.For(ctx).Infof("updating token in db")
	if err := tokenRepo.UpdateByTokenIdentifiersUnsafe(ctx, t.TokenID, contractAddress, t.Chain, up); err != nil {
		logger.For(ctx).WithError(err).Errorf("error updating media")
		return err
	}

	logger.For(ctx).Infof("db update took %s", time.Since(totalUpdateTime))
	logger.For(ctx).Infof("total time took %s", time.Since(totalTime))
	return nil
}

func processOwnersForContractTokens(mc *multichain.Provider, contractRepo persist.ContractGalleryRepository, throttler *throttle.Locker) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input task.TokenProcessingContractTokensMessage
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}
		contract, err := contractRepo.GetByID(c, input.ContractID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		key := fmt.Sprintf("%s-%d", contract.Address, contract.Chain)

		if !input.ForceRefresh {
			if err := throttler.Lock(c, key); err != nil {
				util.ErrResponse(c, http.StatusOK, err)
				return
			}
		}

		// do not unlock, let expiry handle the unlock
		logger.For(c).Infof("processing collection refresh: %s")
		if err := mc.RefreshTokensForContract(c, contract.ContractIdentifiers()); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		logger.For(c).Infof("finished collection refresh: %s", key)

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}
