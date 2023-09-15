package tokenprocessing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v4"
	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc/pool"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/service/tokenmanage"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
)

type ProcessMediaForTokenInput struct {
	TokenID         persist.TokenID `json:"token_id" binding:"required"`
	ContractAddress persist.Address `json:"contract_address" binding:"required"`
	Chain           persist.Chain   `json:"chain"`
}

func processMediaForUsersTokens(tp *tokenProcessor, tokenRepo *postgres.TokenGalleryRepository, contractRepo *postgres.ContractGalleryRepository, tm *tokenmanage.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input task.TokenProcessingUserMessage
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		reqCtx := logger.NewContextWithFields(c.Request.Context(), logrus.Fields{"userID": input.UserID})

		wp := pool.New().WithMaxGoroutines(50).WithErrors()

		logger.For(reqCtx).Infof("Processing Media: %s - Started (%d tokens)", input.UserID, len(input.TokenIDs))

		for _, tokenID := range input.TokenIDs {
			tokenID := tokenID

			wp.Go(func() error {
				token, err := tokenRepo.GetByID(reqCtx, tokenID)
				if err != nil {
					return err
				}

				contract, err := contractRepo.GetByID(reqCtx, token.Contract)
				if err != nil {
					logger.For(reqCtx).Errorf("error getting contract: %s", err)
				}

				ctx := sentryutil.NewSentryHubContext(reqCtx)
				_, err = processFromInstance(ctx, tp, tm, token, contract, persist.ProcessingCauseSync, 0)
				return err
			})
		}

		wp.Wait()
		logger.For(reqCtx).Infof("Processing Media: %s - Finished", input.UserID)

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processMediaForTokenIdentifiers(tp *tokenProcessor, tokenRepo *postgres.TokenGalleryRepository, contractRepo *postgres.ContractGalleryRepository, userRepo *postgres.UserRepository, walletRepo *postgres.WalletRepository, tm *tokenmanage.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input ProcessMediaForTokenInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		reqCtx := c.Request.Context()

		var token persist.TokenGallery
		tokens, err := tokenRepo.GetByTokenIdentifiers(reqCtx, input.TokenID, input.ContractAddress, input.Chain, 1, 0)
		if err != nil {
			if util.ErrorAs[persist.ErrTokenGalleryNotFoundByIdentifiers](err) {
				util.ErrResponse(c, http.StatusNotFound, err)
				return
			}
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if len(tokens) == 0 {
			util.ErrResponse(c, http.StatusNotFound, persist.ErrTokenGalleryNotFoundByIdentifiers{
				TokenID:         input.TokenID,
				ContractAddress: input.ContractAddress,
				Chain:           input.Chain,
			})
			return
		}

		token = tokens[0]

		contract, err := contractRepo.GetByID(reqCtx, token.Contract)
		if err != nil {
			if util.ErrorAs[persist.ErrContractNotFoundByID](err) {
				util.ErrResponse(c, http.StatusNotFound, err)
				return
			}
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		_, err = processFromInstance(reqCtx, tp, tm, token, contract, persist.ProcessingCauseRefresh, 0)

		if err != nil {
			if util.ErrorAs[ErrBadToken](err) {
				util.ErrResponse(c, http.StatusUnprocessableEntity, err)
				return
			}
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

// processMediaForTokenManaged processes a single token instance. It's only called for tokens that failed during a sync.
func processMediaForTokenManaged(tp *tokenProcessor, tokenRepo *postgres.TokenGalleryRepository, contractRepo *postgres.ContractGalleryRepository, tm *tokenmanage.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input task.TokenProcessingTokenMessage

		// Remove from queue if bad message
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		contract, err := contractRepo.GetByAddress(c, input.Token.ContractAddress, input.Token.Chain)
		if err != nil {
			util.ErrResponse(c, http.StatusNotFound, err)
			return
		}

		if _, err = processFromIdentifiers(c, tp, tm, input.Token, contract, persist.ProcessingCauseSyncRetry, input.Attempts); err != nil {
			// Only log the error, because tokenmanage will handle reprocessing
			logger.For(c).Errorf("error processing token: %s", err)
		}

		// We always return a 200 because retries are managed by the token manager and we don't want the queue retrying the current message.
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

		lockID := fmt.Sprintf("%s-%d", contract.Address, contract.Chain)

		if !input.ForceRefresh {
			if err := throttler.Lock(c, lockID); err != nil {
				util.ErrResponse(c, http.StatusOK, err)
				return
			}
		}

		// do not unlock, let expiry handle the unlock
		logger.For(c).Infof("Processing: %s - Processing Collection Refresh", lockID)
		if err := mc.RefreshTokensForContract(c, contract.ContractIdentifiers()); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		logger.For(c).Infof("Processing: %s - Finished Processing Collection Refresh", lockID)

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processOwnersForUserTokens(mc *multichain.Provider, queries *coredb.Queries, validator *validator.Validate) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input task.TokenProcessingUserTokensMessage
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		logger.For(c).WithFields(logrus.Fields{"user_id": input.UserID, "total_tokens": len(input.TokenIdentifiers), "token_ids": input.TokenIdentifiers}).Infof("Processing: %s - Processing User Tokens Refresh (total: %d)", input.UserID, len(input.TokenIdentifiers))
		newTokens, err := mc.SyncTokensByUserIDAndTokenIdentifiers(c, input.UserID, util.MapKeys(input.TokenIdentifiers))
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if len(newTokens) > 0 {

			for _, token := range newTokens {
				var curTotal persist.HexString
				dbUniqueTokenIDs, err := queries.GetUniqueTokenIdentifiersByTokenID(c, token.ID)
				if err != nil {
					logger.For(c).Errorf("error getting unique token identifiers: %s", err)
					continue
				}
				for _, q := range dbUniqueTokenIDs.OwnerAddresses {
					curTotal = input.TokenIdentifiers[persist.TokenUniqueIdentifiers{
						Chain:           dbUniqueTokenIDs.Chain,
						ContractAddress: dbUniqueTokenIDs.ContractAddress,
						TokenID:         dbUniqueTokenIDs.TokenID,
						OwnerAddress:    persist.Address(q),
					}].Add(curTotal)
				}

				// verify the total is less than or equal to the total in the db
				if curTotal.BigInt().Cmp(token.Quantity.BigInt()) > 0 {
					logger.For(c).Errorf("error: total quantity of tokens in db is greater than total quantity of tokens on chain")
					continue
				}

				// one event per token identifier (grouping ERC-1155s)
				_, err = event.DispatchEvent(c, coredb.Event{
					ID:             persist.GenerateID(),
					ActorID:        persist.DBIDToNullStr(input.UserID),
					ResourceTypeID: persist.ResourceTypeToken,
					SubjectID:      token.ID,
					UserID:         input.UserID,
					TokenID:        token.ID,
					Action:         persist.ActionNewTokensReceived,
					Data: persist.EventData{
						NewTokenID:       token.ID,
						NewTokenQuantity: curTotal,
					},
				}, validator, nil)
				if err != nil {
					logger.For(c).Errorf("error dispatching event: %s", err)
				}
			}
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

// detectSpamContracts refreshes the alchemy_spam_contracts table with marked contracts from Alchemy
func detectSpamContracts(queries *coredb.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var params coredb.InsertSpamContractsParams

		now := time.Now()

		for _, source := range []struct {
			Chain    persist.Chain
			Endpoint string
		}{
			{persist.ChainETH, env.GetString("ALCHEMY_ETH_NFT_API_URL")},
			{persist.ChainPolygon, env.GetString("ALCHEMY_POLYGON_NFT_API_URL")},
		} {
			url, err := url.Parse(source.Endpoint)
			if err != nil {
				panic(err)
			}

			spamEndpoint := url.JoinPath("getSpamContracts")

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

			for _, contract := range body.Contracts {
				params.ID = append(params.ID, persist.GenerateID().String())
				params.Chain = append(params.Chain, int32(source.Chain))
				params.Address = append(params.Address, source.Chain.NormalizeAddress(contract))
				params.CreatedAt = append(params.CreatedAt, now)
				params.IsSpam = append(params.IsSpam, true)
			}

			if len(params.Address) == 0 {
				panic("no spam contracts found")
			}
		}

		err := queries.InsertSpamContracts(c, params)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processWalletRemoval(queries *coredb.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input task.TokenProcessingWalletRemovalMessage
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		logger.For(c).Infof("Processing wallet removal: UserID=%s, WalletIDs=%v", input.UserID, input.WalletIDs)

		// We never actually remove multiple wallets at a time, but our API does allow it. If we do end up
		// processing multiple wallet removals, we'll just process them in a loop here, because tuning the
		// underlying query to handle multiple wallet removals at a time is difficult.
		for _, walletID := range input.WalletIDs {
			err := queries.RemoveWalletFromTokens(c, coredb.RemoveWalletFromTokensParams{
				WalletID: walletID.String(),
				UserID:   input.UserID,
			})

			if err != nil {
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}
		}

		if err := queries.RemoveStaleCreatorStatusFromTokens(c, input.UserID); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processPostPreflight(tp *tokenProcessor, tm *tokenmanage.Manager, q *coredb.Queries, mc *multichain.Provider, contractRepo *postgres.ContractGalleryRepository, userRepo *postgres.UserRepository, tokenRepo *postgres.TokenGalleryRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input task.PostPreflightMessage

		if err := c.ShouldBindJSON(&input); err != nil {
			// Remove from queue if bad message
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		existingMedia, err := q.GetMediaByUserTokenIdentifiers(c, coredb.GetMediaByUserTokenIdentifiersParams{
			UserID:  input.UserID,
			Chain:   input.Token.Chain,
			Address: input.Token.ContractAddress,
			TokenID: input.Token.TokenID,
		})

		if err != nil && err != pgx.ErrNoRows {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		// Process media for the token
		if !existingMedia.TokenMedia.Active {
			contract, err := mc.PollForTokenByTokenIdentifiers(c, input.Token, retry.DefaultRetry)
			if err != nil {
				// Keep retrying until we get the token or reach max retries
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}
			if _, err = processFromIdentifiers(c, tp, tm, input.Token, contract, persist.ProcessingCausePostPreflight, 0); err != nil {
				// Only log the error, because tokenmanage will handle reprocessing
				logger.For(c).Errorf("error in preflight: error processing token: %s", err)
			}
		}

		// Try to sync the user's token
		if input.UserID != "" {
			user, err := userRepo.GetByID(c, input.UserID)
			if err != nil {
				// If the user doesn't exist, remove the message from the queue
				util.ErrResponse(c, http.StatusOK, err)
				return
			}
			_, err = mc.PollForTokenByUserTokenIdentifiers(c, user.ID, user.Wallets, input.Token, retry.DefaultRetry)
			if err != nil {
				// Keep retrying until we get the token or reach max retries
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processFromInstance(ctx context.Context, tp *tokenProcessor, tm *tokenmanage.Manager, token persist.TokenGallery, contract persist.ContractGallery, cause persist.ProcessingCause, attempts int) (coredb.TokenMedia, error) {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"tokenDBID": token.ID})
	t := persist.NewTokenIdentifiers(contract.Address, token.TokenID, token.Chain)
	media, err := processFromIdentifiers(ctx, tp, tm, t, contract, cause, attempts, PipelineOpts.WithTokenInstance(&token))
	return media, err
}

func processFromIdentifiers(ctx context.Context, tp *tokenProcessor, tm *tokenmanage.Manager, token persist.TokenIdentifiers, contract persist.ContractGallery, cause persist.ProcessingCause, attempts int, opts ...PipelineOption) (coredb.TokenMedia, error) {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{
		"tokenID":         token.TokenID,
		"tokenID_base10":  token.TokenID.Base10String(),
		"contractDBID":    contract.ID,
		"contractAddress": contract.Address,
		"chain":           token.Chain,
	})
	err, closing := tm.StartProcessing(ctx, token, attempts)
	if err != nil {
		return coredb.TokenMedia{}, err
	}
	runOpts := append([]PipelineOption{}, addContractRunOptions(contract)...)
	runOpts = append(runOpts, addContextRunOptions(cause)...)
	runOpts = append(runOpts, opts...)
	media, err := tp.ProcessTokenPipeline(ctx, token, contract, cause, runOpts...)
	defer closing(err)
	return media, err
}

// addContextRunOptions adds pipeline options for specific types of runs
func addContextRunOptions(cause persist.ProcessingCause) (opts []PipelineOption) {
	if cause == persist.ProcessingCauseRefresh || cause == persist.ProcessingCauseSyncRetry || cause == persist.ProcessingCausePostPreflight {
		opts = append(opts, PipelineOpts.WithRefreshMetadata())
	}
	return opts
}

// addContractRunOptions adds pipeline options for specific contracts
func addContractRunOptions(contract persist.ContractGallery) (opts []PipelineOption) {
	if contract.Address == eth.EnsAddress {
		opts = append(opts, PipelineOpts.WithProfileImageKey("profile_image"))
	}
	return opts
}
