package tokenprocessing

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/service/persist/postgres"

	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	"github.com/gammazero/workerpool"
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

func processMediaForUsersTokensOfChain(mc *multichain.Provider, tokenRepo *postgres.TokenGalleryRepository, contractRepo *postgres.ContractGalleryRepository, walletRepo persist.WalletRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, throttler *throttle.Locker) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input task.TokenProcessingUserMessage
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		ctx := logger.NewContextWithFields(c, logrus.Fields{"userID": input.UserID})

		if err := throttler.Lock(ctx, input.UserID.String()); err != nil {
			// Reply with a non-200 status so that the message is tried again later on
			util.ErrResponse(c, http.StatusTooManyRequests, err)
			return
		}
		defer throttler.Unlock(ctx, input.UserID.String())

		wp := workerpool.New(100)
		for _, tokenID := range input.TokenIDs {
			t, err := tokenRepo.GetByID(ctx, tokenID)
			if err != nil {
				logger.For(ctx).Errorf("failed to fetch tokenID=%s: %s", tokenID, err)
				continue
			}

			contract, err := contractRepo.GetByID(ctx, t.Contract)
			if err != nil {
				logger.For(ctx).Errorf("Error getting contract: %s", err)
			}

			wp.Submit(func() {
				key := fmt.Sprintf("%s-%s-%d", t.TokenID, contract.Address, t.Chain)
				imageKeywords, animationKeywords := t.Chain.BaseKeywords()
				err := processToken(ctx, key, t, contract.Address, "", mc, ethClient, ipfsClient, arweaveClient, stg, tokenBucket, tokenRepo, imageKeywords, animationKeywords)
				if err != nil {
					logger.For(c).Errorf("Error processing token: %s", err)
				}
			})
		}

		wp.StopWait()
		logger.For(ctx).Infof("Processing Media: %s - Finished", input.UserID)

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processMediaForToken(mc *multichain.Provider, tokenRepo *postgres.TokenGalleryRepository, userRepo *postgres.UserRepository, walletRepo *postgres.WalletRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, throttler *throttle.Locker) gin.HandlerFunc {
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

		ctx := logger.NewContextWithFields(c, logrus.Fields{"userID": user.ID})

		t, err := tokenRepo.GetByFullIdentifiers(ctx, input.TokenID, input.ContractAddress, input.Chain, user.ID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		err = processToken(ctx, key, t, input.ContractAddress, input.OwnerAddress, mc, ethClient, ipfsClient, arweaveClient, stg, tokenBucket, tokenRepo, input.ImageKeywords, input.AnimationKeywords)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processToken(c context.Context, key string, t persist.TokenGallery, contractAddress, ownerAddress persist.Address, mc *multichain.Provider, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, tokenRepo *postgres.TokenGalleryRepository, imageKeywords, animationKeywords []string) error {
	ctx := logger.NewContextWithFields(c, logrus.Fields{
		"tokenDBID":       t.ID,
		"tokenID":         t.TokenID,
		"contractDBID":    t.Contract,
		"contractAddress": contractAddress,
		"chain":           t.Chain,
	})
	totalTime := time.Now()
	ctx, cancel := context.WithTimeout(ctx, time.Hour)
	defer cancel()

	newMetadata := t.TokenMetadata

	mcMetadata, err := mc.GetTokenMetadataByTokenIdentifiers(ctx, contractAddress, t.TokenID, ownerAddress, t.Chain)
	if err != nil {
		logger.For(ctx).Errorf("error getting metadata from chain: %s", err)
	} else if mcMetadata != nil && len(mcMetadata) > 0 {
		newMetadata = mcMetadata
	}

	image, animation := media.KeywordsForChain(t.Chain, imageKeywords, animationKeywords)

	name, description := media.FindNameAndDescription(ctx, newMetadata)

	if name == "" {
		name = t.Name.String()
	}

	if description == "" {
		description = t.Description.String()
	}

	totalTimeOfMedia := time.Now()
	newMedia, err := media.MakePreviewsForMetadata(ctx, newMetadata, contractAddress, persist.TokenID(t.TokenID.String()), t.TokenURI, t.Chain, ipfsClient, arweaveClient, stg, tokenBucket, image, animation)
	if err != nil {
		logger.For(ctx).Errorf("error processing media for %s: %s", key, err)
		newMedia = persist.Media{
			MediaType: persist.MediaTypeUnknown,
		}
	}
	logger.For(ctx).Infof("processing media took %s", time.Since(totalTimeOfMedia))

	// Don't replace existing usable media if tokenprocessing failed to get new media
	if t.Media.IsServable() && !newMedia.IsServable() {
		logger.For(ctx).Debugf("not replacing existing media for %s: cur %v new %v", key, t.Media.IsServable(), newMedia.IsServable())
		return nil
	}

	if !persist.TokenURI(newMedia.ThumbnailURL).IsRenderable() && persist.TokenURI(t.Media.ThumbnailURL).IsRenderable() {
		newMedia.ThumbnailURL = t.Media.ThumbnailURL
	}

	up := persist.TokenUpdateAllURIDerivedFieldsInput{
		Media:       newMedia,
		Metadata:    newMetadata,
		Name:        persist.NullString(name),
		Description: persist.NullString(description),
		LastUpdated: persist.LastUpdatedTime{},
	}
	if err := tokenRepo.UpdateByTokenIdentifiersUnsafe(ctx, t.TokenID, contractAddress, t.Chain, up); err != nil {
		logger.For(ctx).Errorf("error updating media for %s-%s-%d: %s", t.TokenID, contractAddress, t.Chain, err)
		return err
	}

	logger.For(ctx).Infof("total processing took %s", time.Since(totalTime))
	return nil
}

func processOwnersForContractTokens(mc *multichain.Provider, contractRepo *postgres.ContractGalleryRepository, throttler *throttle.Locker) gin.HandlerFunc {
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
		logger.For(c).Infof("Processing: %s - Processing Collection Refresh", key)
		if err := mc.RefreshTokensForContract(c, contract.ContractIdentifiers()); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		logger.For(c).Infof("Processing: %s - Finished Processing Collection Refresh", key)

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}
