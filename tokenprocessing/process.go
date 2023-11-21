package tokenprocessing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4"
	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc/pool"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/event"
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

func processBatch(tp *tokenProcessor, queries *coredb.Queries, tm *tokenmanage.Manager) gin.HandlerFunc {
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
				_, err = runManagedPipeline(ctx, tp, tm, td, persist.ProcessingCauseSync, 0, addIsSpamJobOption(c))
				return err
			})
		}

		wp.Wait()
		logger.For(reqCtx).Infof("Processing Batch: %s - Finished", input.BatchID)

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processMediaForTokenIdentifiers(tp *tokenProcessor, queries *coredb.Queries, tm *tokenmanage.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input ProcessMediaForTokenInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		td, err := queries.GetTokenDefinitionByTokenIdentifiers(c, coredb.GetTokenDefinitionByTokenIdentifiersParams{
			Chain:           input.Chain,
			ContractAddress: input.ContractAddress,
			TokenID:         input.TokenID,
		})
		if err == pgx.ErrNoRows {
			util.ErrResponse(c, http.StatusNotFound, err)
			return
		}
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		contract, err := queries.GetContractByID(c, td.ContractID)
		if err == pgx.ErrNoRows {
			util.ErrResponse(c, http.StatusNotFound, err)
			return
		}
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		_, err = runManagedPipeline(c, tp, tm, td, persist.ProcessingCauseRefresh, 0, addIsSpamJobOption(contract))

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
func processMediaForTokenManaged(tp *tokenProcessor, queries *coredb.Queries, tm *tokenmanage.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input task.TokenProcessingTokenMessage

		// Remove from queue if bad message
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		td, err := queries.GetTokenDefinitionById(c, input.TokenDefinitionID)
		if err == pgx.ErrNoRows {
			// Remove from queue if not found
			util.ErrResponse(c, http.StatusOK, err)
			return
		}
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		contract, err := queries.GetContractByID(c, td.ContractID)
		if err == pgx.ErrNoRows {
			// Remove from queue if not found
			util.ErrResponse(c, http.StatusOK, err)
			return
		}
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		runManagedPipeline(c, tp, tm, td, persist.ProcessingCauseSyncRetry, input.Attempts, addIsSpamJobOption(contract))

		// We always return a 200 because retries are managed by the token manager and we don't want the queue retrying the current message.
		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processOwnersForContractTokens(mc *multichain.Provider, throttler *throttle.Locker) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input task.TokenProcessingContractTokensMessage
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		contract, err := mc.Queries.GetContractByID(c, input.ContractID)
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
		cID := persist.NewContractIdentifiers(contract.Address, contract.Chain)
		if err := mc.RefreshTokensForContract(c, cID); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		logger.For(c).Infof("Processing: %s - Finished Processing Collection Refresh", lockID)

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processOwnersForUserTokens(mc *multichain.Provider, queries *coredb.Queries) gin.HandlerFunc {
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
				dbUniqueTokenIDs, err := queries.GetUniqueTokenIdentifiersByTokenID(c, token.Instance.ID)
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
				if curTotal.BigInt().Cmp(token.Instance.Quantity.BigInt()) > 0 {
					logger.For(c).Errorf("error: total quantity of tokens in db is greater than total quantity of tokens on chain")
					continue
				}

				// one event per token identifier (grouping ERC-1155s)
				err = event.Dispatch(c, coredb.Event{
					ID:             persist.GenerateID(),
					ActorID:        persist.DBIDToNullStr(input.UserID),
					ResourceTypeID: persist.ResourceTypeToken,
					SubjectID:      token.Instance.ID,
					UserID:         input.UserID,
					TokenID:        token.Instance.ID,
					Action:         persist.ActionNewTokensReceived,
					Data: persist.EventData{
						NewTokenID:       token.Instance.ID,
						NewTokenQuantity: curTotal,
					},
				})
				if err != nil {
					logger.For(c).Errorf("error dispatching event: %s", err)
				}
			}
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

/*
Alchemy Webhook response example:
{
  "webhookId": "wh_v394g727u681i5rj",
  "id": "whevt_13vxrot10y8omrdp",
  "createdAt": "2022-08-03T23:29:11.267808614Z",
  "type": "NFT_ACTIVITY",
  "event": {
    "activity": [
      "network": "ETH_GOERLI",
      {
        "fromAddress": "0x2acc2dff0c1fa9c1c62f518c9415a0ca60e03f77",
        "toAddress": "0x15dd13f3c4c5279222b5f09ed1b9e9340ed17185",
        "contractAddress": "0xf4910c763ed4e47a585e2d34baa9a4b611ae448c",
        "blockNum": "0x78b94e",
        "hash": "0x6ca7fed3e3ca7a97e774b0eab7d8f46b7dcad5b8cf8ff28593a2ba00cdef4bff",
        "erc1155Metadata": [
          {
            "tokenId": "0x2acc2dff0c1fa9c1c62f518c9415a0ca60e03f77000000000000010000000001",
            "value": "0x1"
          }
        ],
        "category": "erc1155",
        "log": {
          "address": "0xf4910c763ed4e47a585e2d34baa9a4b611ae448c",
          "topics": [
            "0xc3d58168c5ae7397731d063d5bbf3d657854427343f4c083240f7aacaa2d0f62",
            "0x0000000000000000000000002acc2dff0c1fa9c1c62f518c9415a0ca60e03f77",
            "0x0000000000000000000000002acc2dff0c1fa9c1c62f518c9415a0ca60e03f77",
            "0x00000000000000000000000015dd13f3c4c5279222b5f09ed1b9e9340ed17185"
          ],
          "data": "0x2acc2dff0c1fa9c1c62f518c9415a0ca60e03f770000000000000100000000010000000000000000000000000000000000000000000000000000000000000001",
          "blockNumber": "0x78b94e",
          "transactionHash": "0x6ca7fed3e3ca7a97e774b0eab7d8f46b7dcad5b8cf8ff28593a2ba00cdef4bff",
          "transactionIndex": "0x1b",
          "blockHash": "0x4887f8bfbba48b7bff0362c34149d76783feae32f29bff3d98c841bc2ba1902f",
          "logIndex": "0x16",
          "removed": false
        }
      }
    ]
}
*/

type AlchemyNFTActivityWebhookResponse struct {
	WebhookID string `json:"webhookId"`
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
	Type      string `json:"type"`
	Event     struct {
		Network  string               `json:"network"`
		Activity []AlchemyNFTActivity `json:"activity"`
	} `json:"event"`
}

type AlchemyNFTActivity struct {
	Category        string          `json:"category"`
	FromAddress     persist.Address `json:"fromAddress"`
	ToAddress       persist.Address `json:"toAddress"`
	ERC721TokenID   persist.TokenID `json:"erc721TokenId"`
	TokenID         persist.TokenID `json:"tokenId"`
	ContractAddress persist.Address `json:"contractAddress"`
	ERC1155Metadata []struct {
		TokenID persist.TokenID   `json:"tokenId"`
		Value   persist.HexString `json:"value"`
	} `json:"erc1155Metadata"`
}

type alchemyTokenIdentifiers struct {
	Chain           persist.Chain
	ContractAddress persist.Address
	TokenID         persist.TokenID
	OwnerAddress    persist.Address
	Quantity        persist.HexString
}

var alchemyIPs = []string{
	"54.236.136.17",
	"34.237.24.169",
}

func processOwnersForAlchemyTokens(mc *multichain.Provider, queries *coredb.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {

		if !util.Contains(alchemyIPs, c.ClientIP()) {
			util.ErrResponse(c, http.StatusUnauthorized, fmt.Errorf("invalid alchemy ip"))
			return
		}

		bodyBytes, err := c.GetRawData()
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		var input AlchemyNFTActivityWebhookResponse
		if err := json.Unmarshal(bodyBytes, &input); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		var chain persist.Chain
		switch input.Event.Network {
		case "ETH_MAINNET", "ETH_GOERLI":
			chain = persist.ChainETH
		case "ARB_MAINNET":
			chain = persist.ChainArbitrum
		default:
			logger.For(c).Errorf("invalid alchemy network: %s", input.Event.Network)
			util.ErrResponse(c, http.StatusInternalServerError, fmt.Errorf("invalid alchemy network: %s", input.Event.Network))
			return
		}

		signature := c.GetHeader("X-Alchemy-Signature")

		if !isValidAlchemySignatureForStringBody(bodyBytes, signature, chain) {
			util.ErrResponse(c, http.StatusUnauthorized, fmt.Errorf("invalid alchemy signature"))
			return
		}

		usersToTokens := make(map[persist.DBID][]alchemyTokenIdentifiers)
		addressToUsers := make(map[persist.ChainAddress]persist.DBID)
		tokenQuantities := make(map[persist.TokenUniqueIdentifiers]persist.HexString)

		for _, activity := range input.Event.Activity {

			chain := persist.ChainETH
			switch input.Event.Network {
			case "ETH_MAINNET", "ETH_GOERLI":
				chain = persist.ChainETH
			case "POLYGON_MAINNET":
				chain = persist.ChainPolygon
			case "ARB_MAINNET":
				chain = persist.ChainArbitrum
			case "OPTIMISM_MAINNET":
				chain = persist.ChainOptimism
			default:
				util.ErrResponse(c, http.StatusInternalServerError, fmt.Errorf("invalid alchemy network: %s", input.Event.Network))
				return
			}

			userID, ok := addressToUsers[persist.NewChainAddress(activity.ToAddress, chain)]
			if !ok {
				user, err := queries.GetUserByAddressAndL1(c, coredb.GetUserByAddressAndL1Params{
					Address: persist.Address(chain.NormalizeAddress(activity.ToAddress)),
					L1Chain: persist.L1Chain(persist.ChainETH),
				})
				if err != nil {
					logger.For(c).Warnf("error getting user by address: %s", err)
					continue
				}

				if user.ID == "" {
					logger.For(c).Warnf("user not found for address: %s", activity.ToAddress)
					continue
				}

				userID = user.ID
			}

			addressToUsers[persist.NewChainAddress(activity.ToAddress, chain)] = userID

			if activity.Category == "erc721" {
				usersToTokens[userID] = append(usersToTokens[userID], alchemyTokenIdentifiers{
					Chain:           chain,
					ContractAddress: activity.ContractAddress,
					TokenID:         activity.ERC721TokenID,
					OwnerAddress:    activity.ToAddress,
					Quantity:        "1",
				})

				tokenQuantities[persist.TokenUniqueIdentifiers{
					Chain:           chain,
					ContractAddress: activity.ContractAddress,
					TokenID:         activity.ERC721TokenID,
					OwnerAddress:    activity.ToAddress,
				}] = "1"
			} else {
				for _, m := range activity.ERC1155Metadata {
					usersToTokens[userID] = append(usersToTokens[userID], alchemyTokenIdentifiers{
						Chain:           chain,
						ContractAddress: activity.ContractAddress,
						TokenID:         m.TokenID,
						OwnerAddress:    activity.ToAddress,
						Quantity:        m.Value,
					})

					tids := persist.TokenUniqueIdentifiers{
						Chain:           chain,
						ContractAddress: activity.ContractAddress,
						TokenID:         m.TokenID,
						OwnerAddress:    activity.ToAddress,
					}
					cur, ok := tokenQuantities[tids]
					if !ok {
						cur = "0"
					}

					tokenQuantities[tids] = m.Value.Add(cur)
				}
			}

		}

		for userID, tokens := range usersToTokens {
			l := logger.For(c).WithFields(logrus.Fields{"user_id": userID, "total_tokens": len(tokens), "token_ids": tokens, "raw_body": string(bodyBytes)})
			l.Infof("Processing: %s - Processing Alchemy User Tokens Refresh (total: %d)", userID, len(tokens))
			newTokens, err := mc.SyncTokensByUserIDAndTokenIdentifiers(c, userID, util.MapWithoutError(tokens, func(t alchemyTokenIdentifiers) persist.TokenUniqueIdentifiers {
				return persist.TokenUniqueIdentifiers{
					Chain:           t.Chain,
					ContractAddress: t.ContractAddress,
					TokenID:         t.TokenID,
					OwnerAddress:    t.OwnerAddress,
				}
			}))
			if err != nil {
				l.Errorf("error syncing tokens: %s", err)
				continue
			}

			if len(newTokens) > 0 {

				for _, token := range newTokens {
					var curTotal persist.HexString
					dbUniqueTokenIDs, err := queries.GetUniqueTokenIdentifiersByTokenID(c, token.Instance.ID)
					if err != nil {
						logger.For(c).Errorf("error getting unique token identifiers: %s", err)
						continue
					}
					for _, q := range dbUniqueTokenIDs.OwnerAddresses {
						curTotal = tokenQuantities[persist.TokenUniqueIdentifiers{
							Chain:           dbUniqueTokenIDs.Chain,
							ContractAddress: dbUniqueTokenIDs.ContractAddress,
							TokenID:         dbUniqueTokenIDs.TokenID,
							OwnerAddress:    persist.Address(q),
						}].Add(curTotal)
					}

					// verify the total is less than or equal to the total in the db
					if curTotal.BigInt().Cmp(token.Instance.Quantity.BigInt()) > 0 {
						l.Errorf("error: total quantity of tokens in db is greater than total quantity of tokens on chain")
						continue
					}

					// one event per token identifier (grouping ERC-1155s)
					err = event.Dispatch(c, coredb.Event{
						ID:             persist.GenerateID(),
						ActorID:        persist.DBIDToNullStr(userID),
						ResourceTypeID: persist.ResourceTypeToken,
						SubjectID:      token.Instance.ID,
						UserID:         userID,
						TokenID:        token.Instance.ID,
						Action:         persist.ActionNewTokensReceived,
						Data: persist.EventData{
							NewTokenID:       token.Instance.ID,
							NewTokenQuantity: curTotal,
						},
					})
					if err != nil {
						l.Errorf("error dispatching event: %s", err)
					}
				}
			}
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func isValidAlchemySignatureForStringBody(
	body []byte, // must be raw string body, not json transformed version of the body
	signature string, // your "X-Alchemy-Signature" from header
	chain persist.Chain,
) bool {
	var secret string
	switch chain {
	case persist.ChainETH:
		secret = env.GetString("ALCHEMY_WEBHOOK_SECRET_ETH")
	case persist.ChainArbitrum:
		secret = env.GetString("ALCHEMY_WEBHOOK_SECRET_ARBITRUM")
	default:
		logger.For(nil).Errorf("invalid chain for alchemy webhook signature: %d", chain)
		return false
	}

	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(body))
	digest := hex.EncodeToString(h.Sum(nil))
	return digest == signature
}

// detectSpamContracts refreshes the alchemy_spam_contracts table with marked contracts from Alchemy
func detectSpamContracts(queries *coredb.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var params coredb.InsertSpamContractsParams

		now := time.Now()

		seen := make(map[persist.ContractIdentifiers]bool)

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

func processPostPreflight(tp *tokenProcessor, tm *tokenmanage.Manager, mc *multichain.Provider, userRepo *postgres.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input task.PostPreflightMessage

		if err := c.ShouldBindJSON(&input); err != nil {
			// Remove from queue if bad message
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		existingMedia, err := mc.Queries.GetMediaByTokenIdentifiersIgnoringStatus(c, coredb.GetMediaByTokenIdentifiersIgnoringStatusParams{
			Chain:           input.Token.Chain,
			ContractAddress: input.Token.ContractAddress,
			TokenID:         input.Token.TokenID,
		})
		if err != nil && err != pgx.ErrNoRows {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		// Process media for the token
		if !existingMedia.Active {
			td, err := mc.TokenExists(c, input.Token, retry.DefaultRetry)
			if err != nil {
				// Keep retrying until we get the token or reach max retries
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}

			contract, err := mc.Queries.GetContractByID(c, td.ContractID)
			if err == pgx.ErrNoRows {
				// Remove from queue if not found
				util.ErrResponse(c, http.StatusOK, err)
				return
			}
			if err != nil {
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}

			runManagedPipeline(c, tp, tm, td, persist.ProcessingCausePostPreflight, 0,
				addIsSpamJobOption(contract),
				PipelineOpts.WithRequireImage(), // Require an image if available
			)
		}

		// Try to sync the user's token if a user is provided
		if input.UserID != "" {
			user, err := userRepo.GetByID(c, input.UserID)
			if err != nil {
				// If the user doesn't exist, remove the message from the queue
				util.ErrResponse(c, http.StatusOK, err)
				return
			}
			// Try to sync the user's tokens by searching for the token in each of the user's wallets
			// SyncTokenByUserWalletsAndTokenIdentifiersRetry processes media for the token if it is found
			// so we don't need to run the pipeline again here
			_, err = mc.SyncTokenByUserWalletsAndTokenIdentifiersRetry(c, user, input.Token, retry.DefaultRetry)
			if err != nil {
				// Keep retrying until we get the token or reach max retries
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func runManagedPipeline(ctx context.Context, tp *tokenProcessor, tm *tokenmanage.Manager, td coredb.TokenDefinition, cause persist.ProcessingCause, attempts int, opts ...PipelineOption) (coredb.TokenMedia, error) {
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
	closing, err := tm.StartProcessing(ctx, td.ID, tID, attempts)
	if err != nil {
		return coredb.TokenMedia{}, err
	}
	runOpts := append([]PipelineOption{}, addContractRunOptions(cID)...)
	runOpts = append(runOpts, addContextRunOptions(cause)...)
	runOpts = append(runOpts, PipelineOpts.WithMetadata(td.Metadata))
	runOpts = append(runOpts, opts...)
	media, err := tp.ProcessTokenPipeline(ctx, tID, cID, cause, runOpts...)
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
func addContractRunOptions(contract persist.ContractIdentifiers) (opts []PipelineOption) {
	if contract == ensContract {
		opts = append(opts, PipelineOpts.WithProfileImageKey("profile_image"))
	}
	return opts
}

func addIsSpamJobOption(c coredb.Contract) PipelineOption {
	return PipelineOpts.WithIsSpamJob(c.IsProviderMarkedSpam)
}
