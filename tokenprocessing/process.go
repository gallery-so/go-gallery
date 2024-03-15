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

func processHighlightMintClaim(
	mc *multichain.Provider,
	highlightProvider *highlight.Provider,
	tp *tokenProcessor,
	tm *tokenmanage.Manager,
	attemptsForTxn int,
	pollTimeForTxn time.Duration,
	attemptsForSync int,
	pollTimeForSync time.Duration,
) gin.HandlerFunc {
	mcPipe := *mc
	return func(ctx *gin.Context) {
		var msg task.HighlightMintClaimMessage

		// Remove from queue if bad message
		if err := ctx.ShouldBindJSON(&msg); err != nil {
			util.ErrResponse(ctx, http.StatusOK, err)
			return
		}

		claim, err := mcPipe.Queries.GetHighlightMintClaim(ctx, msg.ClaimID)
		if err != nil {
			// Remove from queue if non-existent claim
			logger.For(ctx).Warnf("removing non-existent mint claimID=%s from queue", msg.ClaimID)
			util.ErrResponse(ctx, http.StatusOK, err)
			return
		}

		switch claim.Status {
		// Shouldn't never receive claims in these states, but gracefully handle them anyway
		case highlight.ClaimStatusFailedUnknownStatus, highlight.ClaimStatusTxFailed, highlight.ClaimStatusTokenSyncCompleted:
			logger.For(ctx).Infof("highlight mint claimID=%s has status [%s], removing from queue", claim.ID, claim.Status)
			ctx.JSON(http.StatusOK, util.SuccessResponse{Success: true})
			return
		case highlight.ClaimStatusTxPending, highlight.ClaimStatusTxSucceeded:
			// Poll until the transaction completes or until all retries have been used
			status, tokenID, metadata, err := pollHighlightTransaction(ctx, mcPipe.Queries, highlightProvider, claim, attemptsForTxn, pollTimeForTxn)
			if err != nil {
				// Transaction is still executing
				if errors.Is(err, errHighlightTxStillPending) {
					logger.For(ctx).Infof("hightlight mint claimID=%s transaction is still pending, retrying later", claim.ID)
					util.ErrResponse(ctx, http.StatusInternalServerError, fmt.Errorf("claimID=%s transaction still pending", claim.ID))
					return
				}
				// Otherwise the transaction failed or got an unexpected error from highlight, remove from queue
				updateHighlightClaimStatus(ctx, mcPipe.Queries, claim.ID, status, err.Error())
				logger.For(ctx).Errorf("highlight mint claimID=%s transaction failed with status=%s: %s", claim.ID, status, err)
				sentryutil.ReportError(ctx, err)
				util.ErrResponse(ctx, http.StatusOK, err)
				return
			}

			// Transaction succeeded at this point.
			// The default behavior of syncing is to send new tokens to tokenprocessing via a queue, but
			// we want finer control of how and when a token is processed in order to keep track of the
			// minting state, so the submit function is updated to run the tokenprocessing pipeline immediately.
			mcPipe.SubmitTokens = runPipelineImmediatelyForHighlight(tp, tm, mcPipe.Queries, claim.ID, metadata)
			newTokenID := persist.TokenUniqueIdentifiers{TokenID: tokenID.ToHexTokenID(), ContractAddress: claim.ContractAddress, Chain: claim.Chain}

			var newTokens []operation.TokenFullDetails
			var syncErr error

			// We retry here because there is some delay from when the transaction completes to when the token is indexed by providers.
			for i := 0; i < attemptsForSync; i++ {
				newTokens, syncErr = mcPipe.AddTokensToUserUnchecked(ctx, claim.UserID, []persist.TokenUniqueIdentifiers{newTokenID}, []persist.HexString{"1"})
				if err != nil {
					err := fmt.Errorf("failed to sync token for highlight mint claimID=%s, retrying on err: %s; waiting %s (attempt=%d/%d)", claim.ID, err, pollTimeForSync, i, attemptsForSync)
					logger.For(ctx).Error(err)
					<-time.After(pollTimeForSync)
					continue
				}
				break
			}

			// Providers don't have the token yet, put back in the queue to try again later
			if syncErr != nil {
				logger.For(ctx).Warnf("unable to find minted token=%s from providers for claimID=%s, retrying later: %s", newTokenID, claim.ID, syncErr)
				util.ErrResponse(ctx, http.StatusInternalServerError, syncErr)
				return
			}

			var newToken db.Token

			// Token may have been synced - possibly by optimistic syncing, verify that the token does exist
			if len(newTokens) <= 0 {
				token, err := mcPipe.Queries.GetTokenByUserTokenIdentifiers(ctx, db.GetTokenByUserTokenIdentifiersParams{
					OwnerID:         claim.UserID,
					TokenID:         tokenID.ToHexTokenID(),
					Chain:           claim.Chain,
					ContractAddress: claim.ContractAddress,
				})
				if err != nil {
					logger.For(ctx).Errorf("token should already exist for highlight mint claimID=%s, but was not found: %s", claim.ID, err)
					updateHighlightClaimStatus(ctx, mcPipe.Queries, claim.ID, highlight.ClaimStatusFailedUnknownStatus, err.Error())
					sentryutil.ReportError(ctx, err)
					util.ErrResponse(ctx, http.StatusOK, err)
					return
				}
				newToken = token.Token
			} else {
				logger.For(ctx).Infof("synced tokenID=%s for claimID=%s", newToken.ID, claim.ID)
				newToken = newTokens[0].Instance
			}

			// Write complete status
			err = updateHighlightClaimStatusCompleted(ctx, mcPipe.Queries, claim.ID, newToken.ID)
			if err != nil {
				logger.For(ctx).Errorf("unexpected status writing success status for claimID=%s: %s", claim.ID, err)
				updateHighlightClaimStatus(ctx, mcPipe.Queries, claim.ID, highlight.ClaimStatusFailedUnknownStatus, err.Error())
				sentryutil.ReportError(ctx, err)
				util.ErrResponse(ctx, http.StatusOK, err)
				return
			}
			ctx.JSON(http.StatusOK, util.SuccessResponse{Success: true})
			return
		default:
			err := fmt.Errorf("highlight mint claimID=%s has an unexpected status [%s], removing from queue", claim.ID, claim.Status)
			logger.For(ctx).Warn(err)
			sentryutil.ReportError(ctx, err)
			util.ErrResponse(ctx, http.StatusOK, err)
			return
		}
	}
}

func mustExpectedStatus(expected, got highlight.ClaimStatus) {
	if expected != got {
		panic(fmt.Sprintf("unexpected status; expected=%s; got=%s", expected, got))
	}
}

var errHighlightTxStillPending = fmt.Errorf("transaction is still pending")

func pollHighlightTransaction(
	ctx context.Context,
	q *db.Queries,
	highlightProvider *highlight.Provider,
	claim db.HighlightMintClaim,
	attemptsForTxn int,
	pollTimeForTxn time.Duration,
) (highlight.ClaimStatus, persist.DecimalTokenID, persist.TokenMetadata, error) {
	var status highlight.ClaimStatus
	var tokenID persist.DecimalTokenID
	var metadata persist.TokenMetadata
	var err error
	for i := 0; i < attemptsForTxn; i++ {
		status, tokenID, metadata, err = highlightProvider.GetClaimStatus(ctx, claim.ClaimID.String)
		if err != nil {
			return status, "", persist.TokenMetadata{}, err
		}
		if status == highlight.ClaimStatusTxPending {
			updateHighlightClaimStatus(ctx, q, claim.ID, status, "")
			logger.For(ctx).Infof("claimID=%s transaction still pending, waiting %s (attempt=%d/%d)", claim.ID, pollTimeForTxn, i, attemptsForTxn)
			<-time.After(pollTimeForTxn)
			continue
		}
		mustExpectedStatus(highlight.ClaimStatusTxSucceeded, status)
		updateHighlightClaimStatus(ctx, q, claim.ID, status, "")
		return status, tokenID, metadata, nil
	}
	mustExpectedStatus(highlight.ClaimStatusTxPending, status)
	updateHighlightClaimStatus(ctx, q, claim.ID, status, "")
	return status, "", persist.TokenMetadata{}, errHighlightTxStillPending
}

func updateHighlightClaimStatusCompleted(ctx context.Context, q *db.Queries, claimID, tokenID persist.DBID) error {
	return q.UpdateHighlightMintClaimStatusCompleted(ctx, db.UpdateHighlightMintClaimStatusCompletedParams{
		Status:  highlight.ClaimStatusTokenSyncCompleted,
		TokenID: tokenID,
		ID:      claimID,
	})
}

func updateHighlightClaimStatus(ctx context.Context, q *db.Queries, claimID persist.DBID, status highlight.ClaimStatus, errorMsg string) {
	err := q.UpdateHighlightMintClaimStatus(ctx, db.UpdateHighlightMintClaimStatusParams{
		ID:           claimID,
		Status:       status,
		ErrorMessage: util.ToNullStringEmptyNull(errorMsg),
	})
	if err != nil {
		logger.For(ctx).Errorf("highlight mint claimID=%s failed to update claim status: %s", claimID, err)
		sentryutil.ReportError(ctx, err)
	}
}

func runPipelineImmediatelyForHighlight(tp *tokenProcessor, tm *tokenmanage.Manager, q *db.Queries, claimID persist.DBID, metadata persist.TokenMetadata) multichain.SubmitTokensF {
	return func(ctx context.Context, tDefIDs []persist.DBID) error {
		tm.ProcessRegistry.SetEnqueue(ctx, tDefIDs[0])

		td, err := q.GetTokenDefinitionById(ctx, tDefIDs[0])
		if err != nil {
			return err
		}

		c, err := q.GetContractByID(ctx, td.ContractID)
		if err != nil {
			return err
		}

		_, err = runManagedPipeline(ctx, tp, tm, td, c, persist.ProcessingCauseAppMint, 0,
			PipelineOpts.WithRequireImage(),
			PipelineOpts.WithMetadata(metadata),
		)

		// runManagedPipeline handles retries, so just the log the error
		if err != nil {
			logger.For(ctx).Errorf("pipeline failed for highlight mint claimID=%s: %s", err)
		}

		return err
	}
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
