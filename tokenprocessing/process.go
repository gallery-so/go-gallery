package tokenprocessing

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/sourcegraph/conc/pool"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
	"github.com/sirupsen/logrus"
)

type ProcessMediaForTokenInput struct {
	TokenID         persist.TokenID `json:"token_id" binding:"required"`
	ContractAddress persist.Address `json:"contract_address" binding:"required"`
	Chain           persist.Chain   `json:"chain"`
	OwnerAddress    persist.Address `json:"owner_address" binding:"required"`
}

func processMediaForUsersTokens(tp *tokenProcessor, tokenRepo *postgres.TokenGalleryRepository, contractRepo *postgres.ContractGalleryRepository, throttler *throttle.Locker) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input task.TokenProcessingUserMessage
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		reqCtx := logger.NewContextWithFields(c.Request.Context(), logrus.Fields{"userID": input.UserID})

		if err := throttler.Lock(reqCtx, input.UserID.String()); err != nil {
			// Reply with a non-200 status so that the message is tried again later on
			util.ErrResponse(c, http.StatusTooManyRequests, err)
			return
		}
		defer throttler.Unlock(reqCtx, input.UserID.String())

		wp := pool.New().WithMaxGoroutines(50).WithErrors()

		for _, tokenID := range input.TokenIDs {
			t, err := tokenRepo.GetByID(reqCtx, tokenID)
			if err != nil {
				logger.For(reqCtx).Errorf("failed to fetch tokenID=%s: %s", tokenID, err)
				continue
			}

			contract, err := contractRepo.GetByID(reqCtx, t.Contract)
			if err != nil {
				logger.For(reqCtx).Errorf("Error getting contract: %s", err)
			}

			wp.Go(func() error {
				ctx := sentryutil.NewSentryHubContext(reqCtx)
				err := tp.ProcessTokenPipeline(reqCtx, t, contract, "", persist.ProcessingCauseSync)
				if err != nil {

					logger.For(ctx).Errorf("Error processing token: %s", err)

					return err
				}
				return nil
			})
		}

		wp.Wait()
		logger.For(reqCtx).Infof("Processing Media: %s - Finished", input.UserID)

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processMediaForToken(tp *tokenProcessor, tokenRepo *postgres.TokenGalleryRepository, contractRepo persist.ContractGalleryRepository, userRepo *postgres.UserRepository, walletRepo *postgres.WalletRepository, throttler *throttle.Locker) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input ProcessMediaForTokenInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		reqCtx := c.Request.Context()

		lockerKey := fmt.Sprintf("%s-%s-%d", input.TokenID, input.ContractAddress, input.Chain)
		if err := throttler.Lock(reqCtx, lockerKey); err != nil {
			util.ErrResponse(c, http.StatusTooManyRequests, err)
			return
		}
		defer throttler.Unlock(reqCtx, lockerKey)

		wallet, err := walletRepo.GetByChainAddress(reqCtx, persist.NewChainAddress(input.OwnerAddress, input.Chain))
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		user, err := userRepo.GetByWalletID(reqCtx, wallet.ID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		reqCtx = logger.NewContextWithFields(reqCtx, logrus.Fields{"userID": user.ID})

		t, err := tokenRepo.GetByFullIdentifiers(reqCtx, input.TokenID, input.ContractAddress, input.Chain, user.ID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		contract, err := contractRepo.GetByID(reqCtx, t.Contract)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		err = tp.ProcessTokenPipeline(reqCtx, t, contract, input.OwnerAddress, persist.ProcessingCauseRefresh)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
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

// detectSpamContracts refreshes the alchemy_spam_contracts table with marked contracts from Alchemy
func detectSpamContracts(queries *coredb.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		alchemyURL, err := url.Parse(env.GetString("ALCHEMY_NFT_API_URL"))
		if err != nil {
			panic(err)
		}

		spamEndpoint := alchemyURL.JoinPath("getSpamContracts")

		req, err := http.NewRequestWithContext(c, http.MethodGet, spamEndpoint.String(), nil)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		req.Header.Add("accept", "application/json")

		resp, err := retry.RetryRequest(http.DefaultClient, req)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
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
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		params := coredb.InsertSpamContractsParams{}

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
