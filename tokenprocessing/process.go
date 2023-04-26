package tokenprocessing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/sourcegraph/conc/pool"

	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
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

		// if err := throttler.Lock(ctx, input.UserID.String()); err != nil {
		// 	// Reply with a non-200 status so that the message is tried again later on
		// 	util.ErrResponse(c, http.StatusTooManyRequests, err)
		// 	return
		// }
		// defer throttler.Unlock(ctx, input.UserID.String())

		wp := pool.New().WithMaxGoroutines(50).WithContext(ctx)
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

			wp.Go(func(ctx context.Context) error {
				key := fmt.Sprintf("%s-%s-%d", t.TokenID, contract.Address, t.Chain)
				imageKeywords, animationKeywords := t.Chain.BaseKeywords()
				err := processToken(ctx, key, t, contract, "", mc, ethClient, ipfsClient, arweaveClient, stg, tokenBucket, tokenRepo, imageKeywords, animationKeywords, false)
				if err != nil {

					logger.For(c).Errorf("Error processing token: %s", err)

					return err
				}
				return nil
			})
		}

		wp.Wait()
		logger.For(ctx).Infof("Processing Media: %s - Finished", input.UserID)

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processMediaForToken(mc *multichain.Provider, tokenRepo *postgres.TokenGalleryRepository, contractRepo *postgres.ContractGalleryRepository, userRepo *postgres.UserRepository, walletRepo *postgres.WalletRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, throttler *throttle.Locker) gin.HandlerFunc {
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

		contract, err := contractRepo.GetByID(ctx, t.Contract)
		if err != nil {
			logger.For(ctx).Errorf("Error getting contract: %s", err)
		}

		err = processToken(ctx, key, t, contract, input.OwnerAddress, mc, ethClient, ipfsClient, arweaveClient, stg, tokenBucket, tokenRepo, input.ImageKeywords, input.AnimationKeywords, true)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processToken(c context.Context, key string, t persist.TokenGallery, contract persist.ContractGallery, ownerAddress persist.Address, mc *multichain.Provider, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, tokenRepo *postgres.TokenGalleryRepository, imageKeywords, animationKeywords []string, forceRefresh bool) error {
	// runID is a unique ID for this run of the pipeline
	runID := persist.GenerateID()

	loggerCtx := logger.NewContextWithFields(c, logrus.Fields{
		"tokenDBID":       t.ID,
		"tokenID":         t.TokenID,
		"contractDBID":    t.Contract,
		"contractAddress": contract.Address,
		"chain":           t.Chain,
		"runID":           runID,
	})
	totalTime := time.Now()
	ctx, cancel := context.WithTimeout(loggerCtx, time.Minute*10)
	defer cancel()

	newMetadata := t.TokenMetadata

	if len(newMetadata) == 0 || forceRefresh {
		mcMetadata, err := mc.GetTokenMetadataByTokenIdentifiers(ctx, contract.Address, t.TokenID, ownerAddress, t.Chain)
		if err != nil {
			logger.For(ctx).Errorf("error getting metadata from chain: %s", err)
		} else if mcMetadata != nil && len(mcMetadata) > 0 {
			logger.For(ctx).Infof("got metadata from chain: %v", mcMetadata)
			newMetadata = mcMetadata
		}
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
	newMedia, err := media.MakePreviewsForMetadata(ctx, newMetadata, contract.Address, persist.TokenID(t.TokenID.String()), t.TokenURI, t.Chain, ipfsClient, arweaveClient, stg, tokenBucket, image, animation)
	if err != nil {
		logger.For(ctx).Errorf("error processing media for %s: %s", key, err)
		isSpam := contract.IsProviderMarkedSpam || util.GetOptionalValue(t.IsProviderMarkedSpam, false) || util.GetOptionalValue(t.IsUserMarkedSpam, false)
		sentryutil.ReportTokenError(ctx, err, runID, t.Chain, contract.Address, t.TokenID, isSpam)
		if newMedia.MediaType != persist.MediaTypeInvalid {
			newMedia = persist.Media{
				MediaType: persist.MediaTypeUnknown,
			}
		}
	}
	logger.For(ctx).Infof("processing media took %s", time.Since(totalTimeOfMedia))

	// Don't replace existing usable media if tokenprocessing failed to get new media
	// In the near future we will establish this state explicitly in the DB or in logs
	if t.Media.IsServable() && !newMedia.IsServable() {
		logger.For(ctx).Debugf("not replacing existing media for %s: cur %v new %v", key, t.Media.IsServable(), newMedia.IsServable())
		return nil
	}

	up := persist.TokenUpdateAllURIDerivedFieldsInput{
		Media:       newMedia,
		Metadata:    newMetadata,
		Name:        persist.NullString(name),
		Description: persist.NullString(description),
		LastUpdated: persist.LastUpdatedTime{},
	}
	if err := tokenRepo.UpdateByTokenIdentifiersUnsafe(ctx, t.TokenID, contract.Address, t.Chain, up); err != nil {
		logger.For(ctx).Errorf("error updating media for %s-%s-%d: %s", t.TokenID, contract.Address, t.Chain, err)
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

func detectSpamContracts(queries *db.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		alchemyURL, err := url.Parse(env.GetString("ALCHEMY_NFT_API_URL"))
		if err != nil {
			panic(err)
		}

		spamEndpoint := alchemyURL.JoinPath("getSpamContracts")

		req, err := http.NewRequestWithContext(c, http.MethodGet, spamEndpoint.String(), nil)
		if err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		req.Header.Add("accept", "application/json")

		resp, err := retry.RetryRequest(http.DefaultClient, req)
		if err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			util.ErrResponse(c, http.StatusInternalServerError, util.BodyAsError(resp))
			return
		}

		body := struct {
			Contracts []persist.Address `json:"contractAddresses"`
		}{}

		err = json.NewDecoder(resp.Body).Decode(&body)
		if err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		params := db.InsertSpamContractsParams{}

		now := time.Now()

		for _, contract := range body.Contracts {
			params.ID = append(params.ID, persist.GenerateID().String())
			params.Chain = append(params.Chain, int32(persist.ChainETH))
			params.Address = append(params.Address, persist.ChainETH.NormalizeAddress(contract))
			params.CreatedAt = append(params.CreatedAt, now)
			params.IsSpam = append(params.IsSpam, true)
		}

		if len(params.Address) == 0 {
			panic("no spam contracts found")
		}

		err = queries.InsertSpamContracts(c, params)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}
