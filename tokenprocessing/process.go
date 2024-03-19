package tokenprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4"
	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc/pool"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/platform"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/highlight"
	"github.com/mikeydub/go-gallery/service/multichain/operation"
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
	TokenID         persist.HexTokenID `json:"token_id" binding:"required"`
	ContractAddress persist.Address    `json:"contract_address" binding:"required"`
	Chain           persist.Chain      `json:"chain"`
}

func processBatch(tp *tokenProcessor, queries *db.Queries, taskClient *task.Client, tm *tokenmanage.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input task.TokenProcessingBatchMessage
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		reqCtx := logger.NewContextWithFields(c.Request.Context(), logrus.Fields{"batchID": input.BatchID})

		wp := pool.New().WithMaxGoroutines(50).WithErrors()

		logger.For(reqCtx).Infof("Processing Batch: %s - Started (%d tokens)", input.BatchID, len(input.TokenDefinitionIDs))

		for _, id := range input.TokenDefinitionIDs {
			id := id

			wp.Go(func() error {
				td, err := queries.GetTokenDefinitionById(reqCtx, id)
				if err != nil {
					return err
				}

				c, err := queries.GetContractByID(reqCtx, td.ContractID)
				if err != nil {
					return err
				}

				ctx := sentryutil.NewSentryHubContext(reqCtx)
				_, err = runManagedPipeline(ctx, tp, tm, td, c, persist.ProcessingCauseSync, 0,
					PipelineOpts.WithRequireProhibitionimage(c), // Require image to be processed if Prohibition token
					PipelineOpts.WithRequireFxHashSigned(td, c), // Require token to be signed if it is an FxHash token
				)
				return err
			})
		}

		wp.Wait()
		logger.For(reqCtx).Infof("Processing Batch: %s - Finished", input.BatchID)

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processMediaForTokenIdentifiers(tp *tokenProcessor, queries *db.Queries, tm *tokenmanage.Manager) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var input ProcessMediaForTokenInput
		if err := ctx.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(ctx, http.StatusBadRequest, err)
			return
		}

		td, err := queries.GetTokenDefinitionByTokenIdentifiers(ctx, db.GetTokenDefinitionByTokenIdentifiersParams{
			Chain:           input.Chain,
			ContractAddress: input.ContractAddress,
			TokenID:         input.TokenID,
		})
		if err == pgx.ErrNoRows {
			util.ErrResponse(ctx, http.StatusNotFound, err)
			return
		}
		if err != nil {
			util.ErrResponse(ctx, http.StatusInternalServerError, err)
			return
		}

		c, err := queries.GetContractByID(ctx, td.ContractID)
		if err == pgx.ErrNoRows {
			util.ErrResponse(ctx, http.StatusNotFound, err)
			return
		}
		if err != nil {
			util.ErrResponse(ctx, http.StatusInternalServerError, err)
			return
		}

		_, err = runManagedPipeline(ctx, tp, tm, td, c, persist.ProcessingCauseRefresh, 0,
			PipelineOpts.WithRefreshMetadata(), // Refresh metadata
		)

		if err != nil {
			if util.ErrorIs[tokenmanage.ErrBadToken](err) {
				util.ErrResponse(ctx, http.StatusUnprocessableEntity, err)
				return
			}
			util.ErrResponse(ctx, http.StatusInternalServerError, err)
			return
		}

		ctx.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

// processMediaForTokenManaged processes a single token instance. It's only called for tokens that failed during a sync.
func processMediaForTokenManaged(tp *tokenProcessor, queries *db.Queries, taskClient *task.Client, tm *tokenmanage.Manager) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var input task.TokenProcessingTokenMessage

		// Remove from queue if bad message
		if err := ctx.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(ctx, http.StatusOK, err)
			return
		}

		td, err := queries.GetTokenDefinitionById(ctx, input.TokenDefinitionID)
		if err == pgx.ErrNoRows {
			// Remove from queue if not found
			util.ErrResponse(ctx, http.StatusOK, err)
			return
		}
		if err != nil {
			util.ErrResponse(ctx, http.StatusInternalServerError, err)
			return
		}

		c, err := queries.GetContractByID(ctx, td.ContractID)
		if err == pgx.ErrNoRows {
			// Remove from queue if not found
			util.ErrResponse(ctx, http.StatusOK, err)
			return
		}
		if err != nil {
			util.ErrResponse(ctx, http.StatusInternalServerError, err)
			return
		}

		runManagedPipeline(ctx, tp, tm, td, c, persist.ProcessingCauseSyncRetry, input.Attempts,
			PipelineOpts.WithRefreshMetadata(), // Refresh metadata
		)

		// We always return a 200 because retries are managed by the token manager and we don't want the queue retrying the current message.
		ctx.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

/*
item transferred payload:

 {
  event_type: 'item_transferred',
  payload: {
    chain: 'zora',
    collection: { slug: 'shadeart' },
    event_timestamp: '2024-01-03T20:27:01.000000+00:00',
    from_account: { address: '0x0000000000000000000000000000000000000000' },
    item: {
      chain: [Object],
      metadata: [Object],
      nft_id: 'zora/0x189d1b324600b134d2929e331d1ee275297505c9/1',
      permalink: 'https://opensea.io/assets/zora/0x189d1b324600b134d2929e331d1ee275297505c9/1'
    },
    quantity: 1,
    to_account: { address: '0x8fbbb256bb8bac4ed70a95d054f4007786a69e2b' },
    transaction: {
      hash: '0x50e764e97fee1683b1070b29ddfcccf0bcce7fe2d2aaf95997a2c5bea067ed92',
      timestamp: '2024-01-03T20:27:01.000000+00:00'
    }
  },
  sent_at: '2024-01-03T20:27:05.820128+00:00'
}
*/

func processOwnersForOpenseaTokens(mc *multichain.Provider, queries *db.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {

		var in persist.OpenSeaWebhookInput
		if err := c.ShouldBindJSON(&in); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		incomingToken := in.Payload

		logger.For(c).WithFields(logrus.Fields{"to_address": incomingToken.ToAccount.Address, "token_id": incomingToken.Item.NFTID.TokenID, "contract_address": incomingToken.Item.NFTID.ContractAddress, "chain": incomingToken.Item.NFTID.Chain, "amount": incomingToken.Quantity}).Infof("OPENSEA: %s - Processing Opensea User Tokens Refresh", incomingToken.ToAccount.Address)

		user, _ := queries.GetUserByAddressAndL1(c, db.GetUserByAddressAndL1Params{
			Address: persist.Address(incomingToken.ToAccount.Address.String()),
			L1Chain: incomingToken.Item.NFTID.Chain.L1Chain(),
		})
		if user.ID == "" {
			logger.For(c).Warnf("user not found for address: %s", incomingToken.ToAccount.Address)
			// it is a valid response to not find a user, not every transfer exists on gallery
			c.String(http.StatusOK, fmt.Sprintf("user not found for address: %s", incomingToken.ToAccount.Address))
			return
		}

		beforeToken, _ := queries.GetTokenByUserTokenIdentifiers(c, db.GetTokenByUserTokenIdentifiersParams{
			OwnerID:         user.ID,
			TokenID:         incomingToken.Item.NFTID.TokenID,
			Chain:           incomingToken.Item.NFTID.Chain,
			ContractAddress: incomingToken.Item.NFTID.ContractAddress,
		})

		beforeBalance := persist.HexString("0")
		if beforeToken.Token.ID != "" {
			beforeBalance = beforeToken.Token.Quantity
		}

		l := logger.For(c).WithFields(logrus.Fields{"user_id": user.ID, "token_id": incomingToken.Item.NFTID.TokenID, "contract_address": incomingToken.Item.NFTID.ContractAddress, "chain": incomingToken.Item.NFTID.Chain, "user_address": incomingToken.ToAccount.Address})
		l.Infof("Processing: %s - Processing Opensea User Tokens Refresh", user.ID)
		tokenToAdd := persist.TokenUniqueIdentifiers{
			Chain:           incomingToken.Item.NFTID.Chain,
			ContractAddress: incomingToken.Item.NFTID.ContractAddress,
			TokenID:         incomingToken.Item.NFTID.TokenID,
			OwnerAddress:    persist.Address(incomingToken.ToAccount.Address.String()),
		}
		newQuantity := persist.MustHexString(strconv.Itoa(incomingToken.Quantity))
		newQuantity = newQuantity.Add(beforeBalance)
		newTokens, err := mc.AddTokensToUserUnchecked(c, user.ID, []persist.TokenUniqueIdentifiers{tokenToAdd}, []persist.HexString{newQuantity})
		if err != nil {
			l.Errorf("error syncing tokens: %s", err)
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if len(newTokens) == 0 {
			l.Info("no new tokens found")
		} else {
			l.Infof("found %d new tokens", len(newTokens))
		}

		for _, token := range newTokens {

			dbToken, err := queries.GetUniqueTokenIdentifiersByTokenID(c, token.Instance.ID)
			if err != nil {
				l.Errorf("error getting unique token identifiers: %s", err)
				continue
			}

			newBalance := big.NewInt(0).Sub(dbToken.Quantity.BigInt(), beforeBalance.BigInt())

			if newBalance.Cmp(big.NewInt(0)) <= 0 {
				l.Infof("token quantity is 0 or less, skipping")
				continue
			}

			// one event per token identifier (grouping ERC-1155s)
			err = event.Dispatch(c, db.Event{
				ID:             persist.GenerateID(),
				ActorID:        persist.DBIDToNullStr(user.ID),
				ResourceTypeID: persist.ResourceTypeToken,
				SubjectID:      token.Instance.ID,
				UserID:         user.ID,
				TokenID:        token.Instance.ID,
				Action:         persist.ActionNewTokensReceived,
				Data: persist.EventData{
					NewTokenID:       token.Instance.ID,
					NewTokenQuantity: persist.HexString(newBalance.Text(16)),
				},
			})
			if err != nil {
				l.Errorf("error dispatching event: %s", err)
			}
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

// detectSpamContracts refreshes the alchemy_spam_contracts table with marked contracts from Alchemy
func detectSpamContracts(queries *db.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var params db.InsertSpamContractsParams

		now := time.Now()

		seen := make(map[persist.ContractIdentifiers]bool)

		for _, source := range []struct {
			Chain    persist.Chain
			Endpoint string
		}{
			{persist.ChainETH, env.GetString("ALCHEMY_API_URL")},
			{persist.ChainPolygon, env.GetString("ALCHEMY_POLYGON_API_URL")},
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
				id := persist.NewContractIdentifiers(contract, source.Chain)
				if seen[id] {
					continue
				}
				seen[id] = true
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

func processWalletRemoval(queries *db.Queries) gin.HandlerFunc {
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
			err := queries.RemoveWalletFromTokens(c, db.RemoveWalletFromTokensParams{
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

func processPostPreflight(tp *tokenProcessor, mc *multichain.Provider, userRepo *postgres.UserRepository, taskClient *task.Client, tm *tokenmanage.Manager) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var input task.PostPreflightMessage

		if err := ctx.ShouldBindJSON(&input); err != nil {
			// Remove from queue if bad message
			util.ErrResponse(ctx, http.StatusOK, err)
			return
		}

		existingMedia, err := mc.Queries.GetMediaByTokenIdentifiersIgnoringStatus(ctx, db.GetMediaByTokenIdentifiersIgnoringStatusParams{
			Chain:           input.Token.Chain,
			ContractAddress: input.Token.ContractAddress,
			TokenID:         input.Token.TokenID,
		})
		if err != nil && err != pgx.ErrNoRows {
			util.ErrResponse(ctx, http.StatusOK, err)
			return
		}

		// Process media for the token
		if !existingMedia.Active {
			td, err := mc.TokenExists(ctx, input.Token, retry.DefaultRetry)
			if err != nil {
				// Keep retrying until we get the token or reach max retries
				util.ErrResponse(ctx, http.StatusInternalServerError, err)
				return
			}

			c, err := mc.Queries.GetContractByID(ctx, td.ContractID)
			if err == pgx.ErrNoRows {
				// Remove from queue if not found
				util.ErrResponse(ctx, http.StatusOK, err)
				return
			}
			if err != nil {
				util.ErrResponse(ctx, http.StatusInternalServerError, err)
				return
			}

			runManagedPipeline(ctx, tp, tm, td, c, persist.ProcessingCausePostPreflight, 0,
				PipelineOpts.WithRefreshMetadata(),          // Refresh metadata
				PipelineOpts.WithRequireImage(),             // Require an image if token is Prohibition token
				PipelineOpts.WithRequireFxHashSigned(td, c), // Require token to be signed if it is an FxHash token
			)
		}

		// Try to sync the user's token if a user is provided
		if input.UserID != "" {
			user, err := userRepo.GetByID(ctx, input.UserID)
			if err != nil {
				// If the user doesn't exist, remove the message from the queue
				util.ErrResponse(ctx, http.StatusOK, err)
				return
			}
			// Try to sync the user's tokens by searching for the token in each of the user's wallets
			// SyncTokenByUserWalletsAndTokenIdentifiersRetry processes media for the token if it is found
			// so we don't need to run the pipeline again here
			_, err = mc.SyncTokenByUserWalletsAndTokenIdentifiersRetry(ctx, user.ID, input.Token, retry.DefaultRetry)
			if err != nil {
				// Keep retrying until we get the token or reach max retries
				util.ErrResponse(ctx, http.StatusInternalServerError, err)
				return
			}
		}

		ctx.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processHighlightMintClaim(mc *multichain.Provider, highlightProvider *highlight.Provider, tp *tokenProcessor, tm *tokenmanage.Manager, taskClient *task.Client, maxHandlerAttempts int) gin.HandlerFunc {
	removeMessageFromQueue := func(ctx *gin.Context, err error) {
		util.ErrResponse(ctx, http.StatusOK, err)
	}

	retryMessage := func(ctx *gin.Context, msg task.HighlightMintClaimMessage, err error) {
		msg.Attempts += 1
		taskClient.CreateTaskForHighlightMintClaim(ctx, msg, task.WithDelay(5*time.Second))
		removeMessageFromQueue(ctx, err)
	}

	tracker := highlightTracker{mc.Queries}

	return func(ctx *gin.Context) {
		var msg task.HighlightMintClaimMessage

		// Remove from queue if bad message
		if err := ctx.ShouldBindJSON(&msg); err != nil {
			tracker.setStatusFailed(ctx, msg.ClaimID, err)
			removeMessageFromQueue(ctx, err)
			return
		}

		claim, err := mc.Queries.GetHighlightMintClaim(ctx, msg.ClaimID)
		if err != nil {
			tracker.setStatusFailed(ctx, msg.ClaimID, err)
			removeMessageFromQueue(ctx, err)
			return
		}

		// Stop if we failed too many times processing the claim
		if msg.Attempts >= maxHandlerAttempts {
			logger.For(ctx).Warnf("hightlight mint claimID=%s max retries reached", msg.ClaimID)
			err = fmt.Errorf("max retries reached; last error: %s", claim.ErrorMessage.String)
			sentryutil.ReportError(ctx, err)

			if claim.Status == highlight.ClaimStatusMediaProcessing {
				tracker.setStatus(ctx, msg.ClaimID, highlight.ClaimStatusMediaFailed, err.Error())
				removeMessageFromQueue(ctx, err)
				return
			}

			tracker.setStatusFailed(ctx, msg.ClaimID, err)
			removeMessageFromQueue(ctx, err)
			return
		}

		// Start highlight mint claim processing
		err = trackMint(ctx, mc, tp, tm, highlightProvider, &tracker, claim)
		if errors.Is(err, errHighlightTxStillPending) {
			logger.For(ctx).Infof("hightlight mint claimID=%s transaction is still pending, retrying later", msg.ClaimID)
			tracker.setStatus(ctx, claim.ID, highlight.ClaimStatusTxPending, err.Error())
			retryMessage(ctx, msg, err)
			return
		}
		if errors.Is(err, errHighlightTokenStillSyncing) {
			logger.For(ctx).Infof("hightlight mint claimID=%s is not available from providers yet, retrying later", msg.ClaimID)
			tracker.setStatus(ctx, claim.ID, highlight.ClaimStatusTxSucceeded, err.Error())
			retryMessage(ctx, msg, err)
			return
		}
		if util.ErrorIs[tokenmanage.ErrBadToken](err) {
			logger.For(ctx).Infof("hightlight mint claimID=%s failed media processing, retrying later: %s", msg.ClaimID, err)
			tracker.setStatus(ctx, claim.ID, highlight.ClaimStatusMediaProcessing, err.Error())
			retryMessage(ctx, msg, err)
			return
		}
		if util.ErrorIs[throttle.ErrThrottleLocked](err) {
			logger.For(ctx).Infof("hightlight mint claimID=%s other process holds media processing lock, retrying later", msg.ClaimID)
			tracker.setStatus(ctx, claim.ID, highlight.ClaimStatusMediaProcessing, err.Error())
			retryMessage(ctx, msg, err)
			return
		}
		if err != nil {
			logger.For(ctx).Errorf("highlight mint claimID=%s unexpected error while handling mint: %s", msg.ClaimID, err)
			sentryutil.ReportError(ctx, err)
			tracker.setStatusFailed(ctx, msg.ClaimID, err)
			removeMessageFromQueue(ctx, err)
			return
		}

		// Notify downstream that the claim has been processed
		claim, err = tracker.setStatusMediaProcessed(ctx, claim.ID)
		if err != nil {
			logger.For(ctx).Errorf("highlight mint claimID=%s unexpected error writing success status: %s", msg.ClaimID, err)
			sentryutil.ReportError(ctx, err)
			tracker.setStatusFailed(ctx, msg.ClaimID, err)
			removeMessageFromQueue(ctx, err)
			return
		}

		logger.For(ctx).Infof("succesfully handled highlight mint claimID=%s", msg.ClaimID)
		ctx.JSON(http.StatusOK, util.SuccessResponse{Success: true})
		return
	}
}

var (
	errHighlightTxStillPending    = fmt.Errorf("transaction is still pending")
	errHighlightTokenStillSyncing = fmt.Errorf("minted token is still syncing")
)

func trackMint(ctx context.Context, mc *multichain.Provider, tp *tokenProcessor, tm *tokenmanage.Manager, h *highlight.Provider, tracker *highlightTracker, claim db.HighlightMintClaim) error {
	// The default behavior of SubmitTokens is to send the new token to tokenprocessing by a queue.
	// We want to run the token as soon as we get it and also to accurately track the minting state,
	// so SubmitTokens is updated to a noop so the token isn't processed twice.
	mc = &(*mc)
	mc.SubmitTokens = func(context.Context, []persist.DBID) error { return nil }

	// Guard to protect against the pipeline never exiting if something is buggy with the state machine
	maxDepth := 10

	for i := 0; i <= maxDepth; i++ {
		switch claim.Status {
		case highlight.ClaimStatusTxPending:
			mintedTokenID, metadata, err := waitForMinted(ctx, claim.ID, h, claim.ClaimID, 15, 3*time.Second)
			if err != nil {
				return err
			}
			claim, err = tracker.setStatusTxSucceeded(ctx, claim.ID, mintedTokenID, metadata)
			if err != nil {
				return err
			}
		case highlight.ClaimStatusTxSucceeded:
			recipientWallet, err := mc.Queries.GetWalletByID(ctx, claim.RecipientWalletID)
			if err != nil {
				return err
			}
			mintedToken := persist.TokenUniqueIdentifiers{
				OwnerAddress:    recipientWallet.Address,
				TokenID:         claim.TokenMintID,
				ContractAddress: claim.ContractAddress,
				Chain:           claim.Chain,
			}
			tokenDBID, err := waitForSynced(ctx, mc, claim.UserID, claim.ID, mintedToken, 15, 3*time.Second)
			if err != nil {
				return err
			}
			claim, err = tracker.setStatusMediaProcessing(ctx, claim.ID, tokenDBID)
			if err != nil {
				return err
			}
		case highlight.ClaimStatusMediaProcessing:
			t, err := mc.Queries.GetTokenByUserTokenIdentifiers(ctx, db.GetTokenByUserTokenIdentifiersParams{
				OwnerID:         claim.UserID,
				TokenID:         claim.TokenMintID,
				Chain:           claim.Chain,
				ContractAddress: claim.ContractAddress,
			})
			if err != nil {
				return err
			}
			_, err = runManagedPipeline(ctx, tp, tm, t.TokenDefinition, t.Contract, persist.ProcessingCauseAppMint, 0,
				PipelineOpts.WithRequireImage(),
				PipelineOpts.WithMetadata(claim.TokenMetadata),
			)
			return err
		case highlight.ClaimStatusFailedInternal, highlight.ClaimStatusTxFailed, highlight.ClaimStatusMediaFailed:
			return fmt.Errorf(claim.ErrorMessage.String)
		case highlight.ClaimStatusMediaProcessed:
			return nil
		default:
			err := fmt.Errorf("highlight mint claimID=%s has an unexpected status=%s", claim.ID, claim.Status)
			return err
		}
	}

	return fmt.Errorf("pipeline never exited; something wrong with mint pipeline definition; recursion depth=%d", maxDepth)
}

func waitForMinted(ctx context.Context, claimID persist.DBID, h *highlight.Provider, highlightClaimID string, maxAttempts int, retryDelay time.Duration) (persist.DecimalTokenID, persist.TokenMetadata, error) {
	for i := 0; i < maxAttempts; i++ {
		status, tokenID, metadata, err := h.GetClaimStatus(ctx, highlightClaimID)
		if err != nil {
			return "", persist.TokenMetadata{}, err
		}

		if status == highlight.ClaimStatusTxPending {
			logger.For(ctx).Infof("claimID=%s transaction still pending, waiting %s (attempt=%d/%d)", claimID, retryDelay, i+1, maxAttempts)
			<-time.After(retryDelay)
			continue
		}

		if status != highlight.ClaimStatusTxSucceeded {
			return "", persist.TokenMetadata{}, fmt.Errorf("unexpected status; expected=%s; got=%s", highlight.ClaimStatusTxSucceeded, status)

		}

		return tokenID, metadata, nil
	}
	return "", persist.TokenMetadata{}, errHighlightTxStillPending
}

func waitForSynced(ctx context.Context, mc *multichain.Provider, userID, claimID persist.DBID, mintedToken persist.TokenUniqueIdentifiers, maxAttempts int, retryDelay time.Duration) (persist.DBID, error) {
	var newTokens []operation.TokenFullDetails
	var err error

	// There's some delay from when the transaction completes to when the token is indexed by providers.
	for i := 0; i < maxAttempts; i++ {
		newTokens, err = mc.AddTokensToUserUnchecked(ctx, userID, []persist.TokenUniqueIdentifiers{mintedToken}, []persist.HexString{"1"})
		if err != nil {
			err := fmt.Errorf("failed to sync token for highlight mint claimID=%s, retrying on err: %s; waiting %s (attempt=%d/%d)", claimID, err, retryDelay, i+1, maxAttempts)
			logger.For(ctx).Error(err)
			<-time.After(retryDelay)
			continue
		}
		break
	}

	// Providers don't have the token yet, put back in the queue to try again later
	if err != nil {
		return "", errHighlightTokenStillSyncing
	}

	// Token may have been already been synced, verify that the token does exist
	if len(newTokens) <= 0 {
		existingToken, err := mc.Queries.GetTokenByUserTokenIdentifiers(ctx, db.GetTokenByUserTokenIdentifiersParams{
			OwnerID:         userID,
			TokenID:         mintedToken.TokenID,
			Chain:           mintedToken.Chain,
			ContractAddress: mintedToken.ContractAddress,
		})
		if err != nil {
			return "", err
		}
		return existingToken.Token.ID, nil
	}

	return newTokens[0].Instance.ID, nil
}

type highlightTracker struct{ q *db.Queries }

func (m *highlightTracker) setStatusTxSucceeded(ctx context.Context, claimID persist.DBID, tokenMintID persist.DecimalTokenID, tokenMetadata persist.TokenMetadata) (db.HighlightMintClaim, error) {
	return m.q.UpdateHighlightMintClaimStatusTxSucceeded(ctx, db.UpdateHighlightMintClaimStatusTxSucceededParams{
		Status:        highlight.ClaimStatusTxSucceeded,
		TokenMintID:   tokenMintID.ToHexTokenID(),
		TokenMetadata: tokenMetadata,
		ID:            claimID,
	})
}

func (m *highlightTracker) setStatusMediaProcessing(ctx context.Context, claimID, tokenID persist.DBID) (db.HighlightMintClaim, error) {
	return m.q.UpdateHighlightMintClaimStatusMediaProcessing(ctx, db.UpdateHighlightMintClaimStatusMediaProcessingParams{
		Status:  highlight.ClaimStatusMediaProcessing,
		TokenID: tokenID,
		ID:      claimID,
	})
}

func (m *highlightTracker) setStatusTxPending(ctx context.Context, claimID persist.DBID) (db.HighlightMintClaim, error) {
	return m.setStatus(ctx, claimID, highlight.ClaimStatusTxPending, "")
}

func (m *highlightTracker) setStatusFailed(ctx context.Context, claimID persist.DBID, err error) (db.HighlightMintClaim, error) {
	return m.setStatus(ctx, claimID, highlight.ClaimStatusFailedInternal, err.Error())
}

func (m *highlightTracker) setStatusMediaProcessed(ctx context.Context, claimID persist.DBID) (db.HighlightMintClaim, error) {
	return m.setStatus(ctx, claimID, highlight.ClaimStatusMediaProcessed, "")
}

func (m *highlightTracker) setStatus(ctx context.Context, claimID persist.DBID, status highlight.ClaimStatus, errorMsg string) (db.HighlightMintClaim, error) {
	return m.q.UpdateHighlightMintClaimStatus(ctx, db.UpdateHighlightMintClaimStatusParams{
		ID:           claimID,
		Status:       status,
		ErrorMessage: util.ToNullStringEmptyNull(errorMsg),
	})
}

func runManagedPipeline(ctx context.Context, tp *tokenProcessor, tm *tokenmanage.Manager, td db.TokenDefinition, c db.Contract, cause persist.ProcessingCause, attempts int, opts ...PipelineOption) (db.TokenMedia, error) {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{
		"tokenDefinitionDBID": td.ID,
		"contractDBID":        td.ContractID,
		"tokenID":             td.TokenID,
		"tokenID_base10":      td.TokenID.Base10String(),
		"contractAddress":     td.ContractAddress,
		"chain":               td.Chain,
		"cause":               cause,
	})
	tID := persist.NewTokenIdentifiers(td.ContractAddress, td.TokenID, td.Chain)
	cID := persist.NewContractIdentifiers(td.ContractAddress, td.Chain)
	closing, err := tm.StartProcessing(ctx, td, attempts, cause)
	if err != nil {
		return db.TokenMedia{}, err
	}
	// Runtime options that should be applied to every run
	runOpts := append([]PipelineOption{}, addContractRunOptions(cID)...)
	runOpts = append(runOpts, PipelineOpts.WithMetadata(td.Metadata))
	runOpts = append(runOpts, PipelineOpts.WithKeywords(td))
	runOpts = append(runOpts, PipelineOpts.WithIsFxhash(td.IsFxhash))
	runOpts = append(runOpts, PipelineOpts.WithPlaceholderImageURL(td.FallbackMedia.ImageURL.String()))
	runOpts = append(runOpts, PipelineOpts.WithIsSpamJob(c.IsProviderMarkedSpam))
	runOpts = append(runOpts, opts...)
	media, err := tp.ProcessToken(ctx, tID, cID, cause, runOpts...)
	defer closing(media, err)
	return media, err
}

// addContractRunOptions adds pipeline options for specific contracts
func addContractRunOptions(contract persist.ContractIdentifiers) (opts []PipelineOption) {
	if platform.IsEns(contract.Chain, contract.ContractAddress) {
		opts = append(opts, PipelineOpts.WithProfileImageKey("profile_image"))
	}
	return opts
}
