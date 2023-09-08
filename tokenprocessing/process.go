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
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc/pool"

	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
)

type ProcessMediaForTokenInput struct {
	TokenID         persist.TokenID `json:"token_id" binding:"required"`
	ContractAddress persist.Address `json:"contract_address" binding:"required"`
	Chain           persist.Chain   `json:"chain"`
}

func processMediaForUsersTokens(tp *tokenProcessor, tokenRepo *postgres.TokenGalleryRepository, contractRepo *postgres.ContractGalleryRepository, throttler *throttle.Locker) gin.HandlerFunc {
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

			t, err := tokenRepo.GetByID(reqCtx, tokenID)
			if err != nil {
				logger.For(reqCtx).Errorf("failed to fetch tokenID=%s: %s", tokenID, err)
				continue
			}

			contract, err := contractRepo.GetByID(reqCtx, t.Contract)
			if err != nil {
				logger.For(reqCtx).Errorf("error getting contract: %s", err)
			}

			lockID := tokenID.String()

			wp.Go(func() error {
				if err := throttler.Lock(reqCtx, lockID); err != nil {
					logger.For(reqCtx).Warnf("failed to lock tokenID=%s: %s", tokenID, err)
					return err
				}
				defer throttler.Unlock(reqCtx, lockID)
				ctx := sentryutil.NewSentryHubContext(reqCtx)
				_, err := runPipelineForToken(ctx, tp, t, contract, persist.ProcessingCauseSync)
				return err
			})
		}

		wp.Wait()
		logger.For(reqCtx).Infof("Processing Media: %s - Finished", input.UserID)

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processMediaForToken(tp *tokenProcessor, tokenRepo *postgres.TokenGalleryRepository, contractRepo *postgres.ContractGalleryRepository, userRepo *postgres.UserRepository, walletRepo *postgres.WalletRepository, throttler *throttle.Locker) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input ProcessMediaForTokenInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		reqCtx := c.Request.Context()

		lockID := fmt.Sprintf("%s-%s-%d", input.TokenID, input.ContractAddress, input.Chain)

		if err := throttler.Lock(reqCtx, lockID); err != nil {
			util.ErrResponse(c, http.StatusTooManyRequests, err)
			return
		}
		defer throttler.Unlock(reqCtx, lockID)

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

		_, err = runPipelineForToken(reqCtx, tp, token, contract, persist.ProcessingCauseRefresh, PipelineOpts.WithForceFetchMetadata())
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

func runPipelineForToken(ctx context.Context, tp *tokenProcessor, t persist.TokenGallery, contract persist.ContractGallery, cause persist.ProcessingCause, baseOpts ...PipelineOption) (coredb.TokenMedia, error) {
	opts := append([]PipelineOption{}, baseOpts...)
	opts = append(opts, PipelineOpts.WithTokenInstance(&t))
	opts = append(opts, addContractSpecificOptions(contract)...)
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{
		"tokenDBID":       t.ID,
		"tokenID":         t.TokenID,
		"tokenID_base10":  t.TokenID.Base10String(),
		"contractDBID":    t.Contract,
		"contractAddress": contract.Address,
		"chain":           t.Chain,
	})
	token := persist.NewTokenIdentifiers(contract.Address, t.TokenID, t.Chain)
	return tp.ProcessTokenPipeline(ctx, token, contract, cause, opts...)
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

func processPostPreflight(tp *tokenProcessor, q *coredb.Queries, mc *multichain.Provider, contractRepo *postgres.ContractGalleryRepository, userRepo *postgres.UserRepository, tokenRepo *postgres.TokenGalleryRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input task.PostPreflightMessage
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		existingMedia, err := q.GetMediaByTokenIdentifiers(c, coredb.GetMediaByTokenIdentifiersParams{
			Chain:   input.Token.Chain,
			TokenID: input.Token.TokenID,
			Address: input.Token.ContractAddress,
		})

		if err != nil || !existingMedia.TokenMedia.Active {
			err := runPostPreflightUnauthed(c, tp, input.Token, mc, contractRepo, tokenRepo)
			if err != nil {
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}
		}

		if input.UserID != "" {
			user, err := userRepo.GetByID(c, input.UserID)
			if err != nil {
				util.ErrResponse(c, http.StatusOK, err)
				return
			}
			err = runPostPreflightAuthed(c, tp, user, input.Token, mc, contractRepo)
			if err != nil {
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

// addContractSpecificOptions adds pipeline options for specific contracts
func addContractSpecificOptions(contract persist.ContractGallery) (opts []PipelineOption) {
	if contract.Address == eth.EnsAddress {
		opts = append(opts, PipelineOpts.WithProfileImageKey("profile_image"))
	}
	return opts
}

func runPostPreflightUnauthed(ctx context.Context, tp *tokenProcessor, token persist.TokenIdentifiers, mc *multichain.Provider, contractRepo *postgres.ContractGalleryRepository, tokenRepo *postgres.TokenGalleryRepository) error {
	prelightF := func(ctx context.Context) error {
		_, contractID, err := mc.RefreshTokenDescriptorsByTokenIdentifiers(ctx, persist.TokenIdentifiers{
			TokenID:         token.TokenID,
			Chain:           token.Chain,
			ContractAddress: token.ContractAddress,
		})
		if err != nil {
			return err
		}
		contract, err := contractRepo.GetByID(ctx, contractID)
		if err != nil {
			return err
		}
		_, err = tp.ProcessTokenPipeline(ctx, token, contract, persist.ProcessingCausePostPreflight, PipelineOpts.WithForceFetchMetadata())
		return err
	}

	shouldRetry := func(err error) bool {
		logger.For(ctx).Errorf("error occurred running unauthed preflight, retrying on error: %s", err.Error())
		return true
	}

	return retry.RetryFunc(ctx, prelightF, shouldRetry, retry.DefaultRetry)
}

func runPostPreflightAuthed(ctx context.Context, tp *tokenProcessor, user persist.User, token persist.TokenIdentifiers, mc *multichain.Provider, contractRepo *postgres.ContractGalleryRepository) error {
	preflightF := func(ctx context.Context) error {
		wallets := multichain.MatchingWalletsForChain(user.Wallets, token.Chain, mc.WalletOverrides)
		tokens := util.MapWithoutError(wallets, func(w persist.Address) persist.TokenUniqueIdentifiers {
			return persist.TokenUniqueIdentifiers{
				Chain:           token.Chain,
				ContractAddress: token.ContractAddress,
				TokenID:         token.TokenID,
			}
		})
		syncedTokens, err := mc.SyncTokensByUserIDAndTokenIdentifiers(ctx, user.ID, tokens)
		if err != nil {
			return err
		}
		contract, err := contractRepo.GetByID(ctx, syncedTokens[0].Contract)
		if err != nil {
			return err
		}
		_, err = runPipelineForToken(ctx, tp, syncedTokens[0], contract, persist.ProcessingCausePostPreflight)
		return err
	}

	shouldRetry := func(err error) bool {
		logger.For(ctx).Errorf("error occurred running authed preflight, retrying on error: %s", err.Error())
		return true
	}

	return retry.RetryFunc(ctx, preflightF, shouldRetry, retry.DefaultRetry)
}
