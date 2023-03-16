package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gammazero/workerpool"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/indexer/refresh"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

type manualIndexHandler func(context.Context, persist.TokenID, persist.EthereumAddress, *ethclient.Client) (persist.Token, error)

var errInvalidUpdateMetadataInput = errors.New("must provide either owner_address or token_id and contract_address")

var bigZero = big.NewInt(0)

var customManualIndex = map[persist.EthereumAddress]manualIndexHandler{
	"0xb47e3cd837ddf8e4c57f05d70ab865de6e193bbb": func(ctx context.Context, ti persist.TokenID, ea persist.EthereumAddress, c *ethclient.Client) (persist.Token, error) {
		ct, err := contracts.NewCryptopunksCaller(common.HexToAddress("0xb47e3cd837ddf8e4c57f05d70ab865de6e193bbb"), c)
		if err != nil {
			return persist.Token{}, err
		}
		owner, err := ct.PunkIndexToAddress(&bind.CallOpts{Context: ctx}, ti.BigInt())
		if err != nil {
			return persist.Token{}, err
		}
		return persist.Token{
			Quantity:        "1",
			TokenType:       persist.TokenTypeERC721,
			OwnerAddress:    persist.EthereumAddress(owner.String()),
			ContractAddress: persist.EthereumAddress("0xb47e3cd837ddf8e4c57f05d70ab865de6e193bbb"),
			TokenID:         ti,
		}, nil
	},
}

// UpdateTokenInput is the input for the update media endpoint that will find all of the media content
// for an addresses NFTs and cache it in a storage bucket
type UpdateTokenInput struct {
	OwnerAddress    persist.EthereumAddress `json:"owner_address,omitempty"`
	TokenID         persist.TokenID         `json:"token_id,omitempty"`
	ContractAddress persist.EthereumAddress `json:"contract_address,omitempty"`
	UpdateAll       bool                    `json:"update_all"`
}

type tokenFullUpdate struct {
	TokenDBID       persist.DBID
	TokenID         persist.TokenID
	ContractAddress persist.EthereumAddress
	Update          persist.TokenUpdateURIInput
}

type tokenMetadataFieldsUpdate struct {
	TokenDBID       persist.DBID
	TokenID         persist.TokenID
	ContractAddress persist.EthereumAddress
	Update          persist.TokenUpdateMetadataFieldsInput
}

type getTokensInput struct {
	WalletAddress   persist.EthereumAddress `form:"address"`
	ContractAddress persist.EthereumAddress `form:"contract_address"`
	TokenID         persist.TokenID         `form:"token_id"`
	Offset          int64                   `form:"offset"`
	Limit           int64                   `form:"limit"`
}

type getTokenMetadataInput struct {
	TokenID         persist.TokenID         `form:"token_id" binding:"required"`
	ContractAddress persist.EthereumAddress `form:"contract_address" binding:"required"`
	OwnerAddress    persist.EthereumAddress `form:"address"`
}

// GetTokensOutput is the response of the get tokens handler
type GetTokensOutput struct {
	NFTs      []persist.Token    `json:"nfts"`
	Contracts []persist.Contract `json:"contracts"`
}

type GetTokenMetadataOutput struct {
	Metadata persist.TokenMetadata `json:"metadata"`
}

// ValidateWalletNFTsInput is the input for the validate users NFTs endpoint that will return
// whether what opensea has on a user is the same as what we have in our database
type ValidateWalletNFTsInput struct {
	Wallet persist.EthereumAddress `json:"wallet,omitempty" binding:"required"`
	All    bool                    `json:"all"`
}

// ValidateUsersNFTsOutput is the output of the validate users NFTs endpoint that will return
// whether what opensea has on a user is the same as what we have in our database
type ValidateUsersNFTsOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

func getTokens(queueChan chan<- processTokensInput, nftRepository persist.TokenRepository, contractRepository persist.ContractRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client, arweaveClient *goar.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getTokensInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		if input.WalletAddress == "" && input.ContractAddress == "" && input.TokenID == "" {
			util.ErrResponse(c, http.StatusBadRequest, util.ErrInvalidInput{Reason: "must specify at least one of id, address, contract_address, token_id"})
			return
		}

		tokens, contracts, err := getTokensFromDB(c, input, nftRepository, contractRepository)
		if err != nil {
			status := http.StatusInternalServerError
			if _, ok := err.(persist.ErrTokenNotFoundByTokenIdentifiers); ok {
				status = http.StatusNotFound
			}
			util.ErrResponse(c, status, err)
			return
		}

		key, err := json.Marshal(input)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		go func() {
			queueChan <- processTokensInput{
				key:       string(key),
				tokens:    tokens,
				contracts: contracts,
			}
		}()

		c.JSON(http.StatusOK, GetTokensOutput{NFTs: tokens, Contracts: contracts})
	}
}

func getTokenMetadata(nftRepository persist.TokenRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client, arweaveClient *goar.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getTokenMetadataInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		ctx := logger.NewContextWithFields(c, logrus.Fields{
			"tokenID":         input.TokenID,
			"contractAddress": input.ContractAddress,
		})

		ctx, cancel := context.WithTimeout(ctx, time.Minute*10)
		defer cancel()

		curTokens, err := nftRepository.GetByTokenIdentifiers(ctx, input.TokenID, input.ContractAddress, -1, 0)
		if err != nil {
			if _, ok := err.(persist.ErrTokenNotFoundByTokenIdentifiers); !ok {
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}
		}

		if len(curTokens) == 0 && input.OwnerAddress != "" {
			t, err := manuallyIndexToken(c, input.TokenID, input.ContractAddress, input.OwnerAddress, ethClient, nftRepository)
			if err != nil {
				logger.For(ctx).Error("error manually indexing token", err)
			} else {
				logger.For(ctx).Infof("manually indexed token: %s-%s (token type: %s)", input.ContractAddress, input.TokenID, t.TokenType)
				curTokens = []persist.Token{t}
			}
		}

		firstWithValidTokenURI, ok := util.FindFirst(curTokens, func(t persist.Token) bool {
			return t.TokenURI != ""
		})

		firstWithValidTokenType, _ := util.FindFirst(curTokens, func(t persist.Token) bool {
			return t.TokenType != ""
		})

		newURI := firstWithValidTokenURI.TokenURI

		asEthAddress := persist.EthereumAddress(input.ContractAddress.String())
		handler, hasCustomHandler := uniqueMetadataHandlers[asEthAddress]

		if !ok || newURI == "" || newURI.Type() == persist.URITypeInvalid || newURI.Type() == persist.URITypeUnknown {
			newURI, err = rpc.GetTokenURI(ctx, firstWithValidTokenType.TokenType, input.ContractAddress, input.TokenID, ethClient)
			// It's possible to fetch metadata for some contracts even if URI data is missing.
			if !hasCustomHandler && (err != nil || newURI == "") {
				util.ErrResponse(c, http.StatusInternalServerError, errNoMetadataFound{Contract: input.ContractAddress, TokenID: input.TokenID})
				return
			}
		}

		newMetadata := firstWithValidTokenURI.TokenMetadata

		if hasCustomHandler {
			logger.For(ctx).Infof("Using %v metadata handler for %s", handler, input.ContractAddress)
			u, md, err := handler(ctx, newURI, asEthAddress, input.TokenID, ethClient, ipfsClient, arweaveClient)
			if err != nil {
				logger.For(ctx).Errorf("Error getting metadata from handler: %s", err)
			} else {
				newMetadata = md
				newURI = u
			}
		} else if newURI != "" {
			md, err := rpc.GetMetadataFromURI(ctx, newURI, ipfsClient, arweaveClient)
			if err != nil {
				logger.For(ctx).Errorf("Error getting metadata from URI: %s", err)
			} else {
				newMetadata = md
			}
		}

		if newMetadata == nil || len(newMetadata) == 0 {
			util.ErrResponse(c, http.StatusInternalServerError, errNoMetadataFound{Contract: input.ContractAddress, TokenID: input.TokenID})
			return
		}

		if err := nftRepository.UpdateByTokenIdentifiers(ctx, input.TokenID, input.ContractAddress, persist.TokenUpdateAllURIDerivedFieldsInput{
			Metadata: newMetadata,
			TokenURI: newURI,
		}); err != nil {
			logger.For(ctx).Errorf("Error updating token metadata: %s", err)
		}

		c.JSON(http.StatusOK, GetTokenMetadataOutput{Metadata: newMetadata})
	}
}

type processTokensInput struct {
	key       string
	tokens    []persist.Token
	contracts []persist.Contract
}

func processMissingMetadata(ctx context.Context, inputs <-chan processTokensInput, nftRepository persist.TokenRepository, contractRepository persist.ContractRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client, arweaveClient *goar.Client, storageClient *storage.Client, tokenBucket string, throttler *throttle.Locker) {
	mainPool := workerpool.New(10)
	for input := range inputs {
		i := input
		mainPool.Submit(func() {
			ctx, cancel := context.WithTimeout(ctx, time.Minute*30)
			defer cancel()

			tokensWithoutMetadata := make([]persist.Token, 0, len(i.tokens))
			for _, token := range i.tokens {
				if token.Name == "" || token.Description == "" {
					tokensWithoutMetadata = append(tokensWithoutMetadata, token)
				}
			}

			contractsWithoutMetadata := make([]persist.Contract, 0, len(i.contracts))
			for _, contract := range i.contracts {
				// The contract name is the most important field and the only field we use at the indexer level as far as contract metadata goes,
				// but we may want to consider updating if other fields are empty in the future as well (such as a description if we start retrieving that field from somewhere)
				if contract.Name == "" {
					contractsWithoutMetadata = append(contractsWithoutMetadata, contract)
				}
			}

			subpool := workerpool.New(10)

			// Process contracts with missing metadata
			for _, contract := range contractsWithoutMetadata {
				c := contract
				subpool.Submit(func() {
					ctx := logger.NewContextWithFields(sentryutil.NewSentryHubContext(ctx), logrus.Fields{"contractAddress": c.Address})

					key := contract.Address.String()

					err := throttler.Lock(ctx, key)
					if err != nil {
						logger.For(ctx).Warnf("failed to acquire lock, skipping contract: %s", err)
						return
					}
					defer throttler.Unlock(ctx, key)

					updateInput := UpdateContractMetadataInput{Address: c.Address}

					err = updateMetadataForContract(ctx, updateInput, ethClient, contractRepository)
					if err != nil {
						logEthCallRPCError(logger.For(ctx).WithError(err), err, "failed to update contract metadata")
					}
				})
			}

			// Process tokens with missing metadata
			for _, token := range tokensWithoutMetadata {
				t := token
				subpool.Submit(func() {
					ctx := logger.NewContextWithFields(sentryutil.NewSentryHubContext(ctx), logrus.Fields{
						"tokenDBID":       t.ID,
						"tokenID":         t.TokenID,
						"contractAddress": t.ContractAddress,
					})

					key := fmt.Sprintf("%s-%s-%d", t.TokenID, t.ContractAddress, t.Chain)

					err := throttler.Lock(ctx, key)
					if err != nil {
						logger.For(ctx).Warnf("failed to acquire lock, skipping token: %s", err)
						return
					}
					defer throttler.Unlock(ctx, key)

					err = updateMetadataFieldsForToken(ctx, t.TokenID, t.ContractAddress, t.TokenMetadata, nftRepository)
					if err != nil {
						logger.For(ctx).Errorf("failed to update token metadata fields: %s", err)
					}
				})
			}

			subpool.StopWait()
		})
	}
	mainPool.StopWait()
}

func getTokensFromDB(pCtx context.Context, input *getTokensInput, tokenRepo persist.TokenRepository, contractRepo persist.ContractRepository) ([]persist.Token, []persist.Contract, error) {
	switch {
	case input.WalletAddress != "":
		if input.TokenID != "" && input.ContractAddress != "" {
			token, err := tokenRepo.GetByIdentifiers(pCtx, input.TokenID, input.ContractAddress, input.WalletAddress)
			if err != nil {
				return nil, nil, err
			}
			contract, err := contractRepo.GetByAddress(pCtx, input.ContractAddress)
			if err != nil {
				return nil, nil, err
			}
			return []persist.Token{token}, []persist.Contract{contract}, nil

		} else if input.ContractAddress != "" {
			tokens, contract, err := tokenRepo.GetOwnedByContract(pCtx, input.ContractAddress, input.WalletAddress, input.Limit, input.Offset)
			if err != nil {
				return nil, nil, err
			}
			return tokens, []persist.Contract{contract}, nil
		}
		return tokenRepo.GetByWallet(pCtx, input.WalletAddress, input.Limit, input.Offset)
	case input.TokenID != "" && input.ContractAddress != "":
		if strings.HasPrefix(string(input.TokenID), "0x") {
			input.TokenID = input.TokenID[2:]
		} else {
			input.TokenID = persist.TokenID(input.TokenID.BigInt().Text(16))
		}

		tokens, err := tokenRepo.GetByTokenIdentifiers(pCtx, input.TokenID, input.ContractAddress, input.Limit, input.Offset)
		if err != nil {
			return nil, nil, err
		}
		contract, err := contractRepo.GetByAddress(pCtx, input.ContractAddress)
		if err != nil {
			return nil, nil, err
		}
		return tokens, []persist.Contract{contract}, nil
	case input.ContractAddress != "":
		tokens, err := tokenRepo.GetByContract(pCtx, input.ContractAddress, input.Limit, input.Offset)
		if err != nil {
			return nil, nil, err
		}
		contract, err := contractRepo.GetByAddress(pCtx, input.ContractAddress)
		if err != nil {
			return nil, nil, err
		}
		return tokens, []persist.Contract{contract}, nil
	default:
		return nil, nil, errors.New("must specify at least one of id, address, contract_address, token_id")
	}
}

func validateWalletsNFTs(tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input ValidateWalletNFTsInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		output, err := validateNFTs(c, input, tokenRepository, contractRepository, ethcl, ipfsClient, arweaveClient)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, output)

	}
}

// validateNFTs will validate the NFTs for the wallet passed in when being compared with opensea
func validateNFTs(c context.Context, input ValidateWalletNFTsInput, tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client) (ValidateUsersNFTsOutput, error) {

	currentNFTs, _, err := tokenRepository.GetByWallet(c, input.Wallet, -1, 0)
	if err != nil {
		return ValidateUsersNFTsOutput{}, err
	}

	output := ValidateUsersNFTsOutput{Success: true}

	if input.All {
		newMsg, err := processAccountedForNFTs(c, currentNFTs, tokenRepository, ethcl, ipfsClient, arweaveClient)
		if err != nil {
			return ValidateUsersNFTsOutput{}, err
		}
		output.Message += newMsg
	}

	openseaAssets, err := opensea.FetchAssetsForWallet(c, input.Wallet)
	if err != nil {
		return ValidateUsersNFTsOutput{}, err
	}

	accountedFor := make(map[persist.DBID]bool)
	unaccountedFor := make(map[string]opensea.Asset)

	for _, asset := range openseaAssets {
		af := false
		for _, nft := range currentNFTs {
			if accountedFor[nft.ID] {
				continue
			}
			if asset.Contract.ContractAddress == nft.ContractAddress && asset.TokenID.String() == nft.TokenID.Base10String() {
				accountedFor[nft.ID] = true
				af = true
				break
			}
		}
		if !af {
			unaccountedFor[asset.Contract.ContractAddress.String()+" -- "+asset.TokenID.String()] = asset
		}
	}

	if len(unaccountedFor) > 0 {
		unaccountedForKeys := make([]string, 0, len(unaccountedFor))
		for k := range unaccountedFor {
			unaccountedForKeys = append(unaccountedForKeys, k)
		}
		accountedForKeys := make([]string, 0, len(accountedFor))
		for k := range accountedFor {
			accountedForKeys = append(accountedForKeys, k.String())
		}

		allUnaccountedForAssets := make([]opensea.Asset, 0, len(unaccountedFor))
		for _, asset := range unaccountedFor {
			allUnaccountedForAssets = append(allUnaccountedForAssets, asset)
		}

		if err := processUnaccountedForNFTs(c, allUnaccountedForAssets, input.Wallet, tokenRepository, contractRepository, ethcl, ipfsClient, arweaveClient); err != nil {
			logger.For(c).WithError(err).Error("failed to process unaccounted for NFTs")
			return ValidateUsersNFTsOutput{}, err
		}
		output.Success = false
		output.Message += fmt.Sprintf("user %s has unaccounted for NFTs: %v | accounted for NFTs: %v", input.Wallet, unaccountedForKeys, accountedForKeys)
	}
	return output, nil
}

func processAccountedForNFTs(ctx context.Context, tokens []persist.Token, tokenRepository persist.TokenRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client) (string, error) {
	msgToAdd := ""
	for _, token := range tokens {

		needsUpdate := false

		uri, err := rpc.GetTokenURI(ctx, token.TokenType, token.ContractAddress, token.TokenID, ethcl)
		if err == nil {
			token.TokenURI = uri.ReplaceID(token.TokenID)
			needsUpdate = true
		} else {
			logEntry := logger.For(ctx).WithError(err).WithFields(logrus.Fields{
				"tokenType":       token.TokenType,
				"tokenID":         token.TokenID,
				"contractAddress": token.ContractAddress,
				"rpcCall":         "eth_call",
			})
			logEthCallRPCError(logEntry, err, fmt.Sprintf("failed to get token URI for token %s-%s: %v", token.ContractAddress, token.TokenID, err))
			msgToAdd += fmt.Sprintf("failed to get token URI for token %s-%s: %v\n", token.ContractAddress, token.TokenID, err)
			continue
		}

		metadata, err := rpc.GetMetadataFromURI(ctx, token.TokenURI, ipfsClient, arweaveClient)
		if err == nil {
			token.TokenMetadata = metadata
			needsUpdate = true
		} else {
			logger.For(ctx).WithError(err).WithFields(logrus.Fields{
				"tokenID":         token.TokenID,
				"contractAddress": token.ContractAddress,
			}).Errorf("failed to get token metadata for token %s-%s with uri %s: %v", token.ContractAddress, token.TokenID, token.TokenURI, err)
			msgToAdd += fmt.Sprintf("failed to get token metadata for token %s-%s with uri %s: %v", token.ContractAddress, token.TokenID, token.TokenURI, err)
			continue
		}

		if needsUpdate {
			update := persist.TokenUpdateAllURIDerivedFieldsInput{
				TokenURI:    token.TokenURI,
				Metadata:    token.TokenMetadata,
				Media:       token.Media,
				Name:        token.Name,
				Description: token.Description,
			}

			if err := tokenRepository.UpdateByID(ctx, token.ID, update); err != nil {
				return "", fmt.Errorf("failed to update token %s: %v", token.TokenID, err)
			}
		}

	}
	return msgToAdd, nil
}
func processUnaccountedForNFTs(ctx context.Context, assets []opensea.Asset, address persist.EthereumAddress, tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client) error {
	errChan := make(chan error)
	wp := workerpool.New(10)
	for _, asset := range assets {
		a := asset
		wp.Submit(func() {
			errChan <- refreshTokenMetadatas(ctx, UpdateTokenInput{OwnerAddress: address, TokenID: persist.TokenID(a.TokenID.ToBase16()), ContractAddress: a.Contract.ContractAddress, UpdateAll: true}, tokenRepository, ethcl, ipfsClient, arweaveClient)
		})
	}
	for i := 0; i < len(assets); i++ {
		err := <-errChan
		if err != nil {
			logger.For(ctx).WithError(err).Error("failed to refresh unaccounted token media")
		}
	}

	return nil
}

func updateTokens(tokenRepository persist.TokenRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := UpdateTokenInput{}
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		err := refreshTokenMetadatas(c, input, tokenRepository, ethClient, ipfsClient, arweaveClient)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func processRefreshes(idxr *indexer, storageClient *storage.Client) gin.HandlerFunc {
	events := eventsToTopics(idxr.eventHashes)
	return func(c *gin.Context) {
		filterManager := refresh.NewBlockFilterManager(c, storageClient)
		defer filterManager.Close()

		refreshPool := workerpool.New(refresh.DefaultConfig.DefaultPoolSize)

		message := task.DeepRefreshMessage{}
		if err := c.ShouldBindJSON(&message); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		refreshRange, err := refresh.ResolveRange(message.RefreshRange)
		if err != nil && errors.Is(err, refresh.ErrInvalidRefreshRange) {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		for block := refreshRange[0]; block < refreshRange[1]; block += blocksPerLogsCall {
			b := block
			refreshPool.Submit(func() {
				ctx := sentryutil.NewSentryHubContext(c)

				exists, err := refresh.AddressExists(ctx, filterManager, message.OwnerAddress, b, b+persist.BlockNumber(blocksPerLogsCall))
				if err != nil {
					if err != refresh.ErrNoFilter {
						logger.For(ctx).WithError(err).Info("failed to fetch filter")
					}
					exists = true
				}

				// Don't need the filter anymore
				filterManager.Clear(ctx, b, b+persist.BlockNumber(blocksPerLogsCall))

				if exists {
					transferCh := make(chan []transfersAtBlock)
					plugins := NewTransferPlugins(ctx, idxr.ethClient, idxr.tokenRepo, idxr.addressFilterRepo)
					enabledPlugins := []chan<- PluginMsg{plugins.balances.in, plugins.owners.in, plugins.uris.in}
					go func() {
						ctx := sentryutil.NewSentryHubContext(ctx)
						logs := idxr.fetchLogs(ctx, b, events)
						transfers := filterTransfers(ctx, message, logsToTransfers(ctx, logs))
						transfersAtBlock := transfersToTransfersAtBlock(transfers)
						transferCh <- transfersAtBlock
						close(transferCh)
					}()
					go idxr.processAllTransfers(sentryutil.NewSentryHubContext(ctx), transferCh, enabledPlugins)
					idxr.processTokens(ctx, plugins.uris.out, plugins.owners.out, plugins.previousOwners.out, plugins.balances.out, nil)
				}
			})
		}
		refreshPool.StopWait()
	}
}

// filterTransfers checks each transfer against the input and returns ones that match the criteria.
func filterTransfers(ctx context.Context, m task.DeepRefreshMessage, transfers []rpc.Transfer) []rpc.Transfer {
	hasOwner := func(t rpc.Transfer) bool {
		return t.To.String() == m.OwnerAddress.String() || t.From.String() == m.OwnerAddress.String()
	}
	hasContract := func(t rpc.Transfer) bool {
		return t.ContractAddress.String() == m.ContractAddress.String()
	}
	hasToken := func(t rpc.Transfer) bool {
		return hasContract(t) && (t.TokenID.String() == m.TokenID.String())
	}

	var criteria func(transfer rpc.Transfer) bool

	switch {
	case m.OwnerAddress != "" && m.TokenID != "" && m.ContractAddress != "":
		criteria = func(t rpc.Transfer) bool { return hasOwner(t) && hasToken(t) }
	case m.OwnerAddress != "" && m.ContractAddress != "":
		criteria = func(t rpc.Transfer) bool { return hasOwner(t) && hasContract(t) }
	case m.OwnerAddress != "":
		criteria = hasOwner
	case m.ContractAddress != "" && m.TokenID != "":
		criteria = hasToken
	case m.ContractAddress != "":
		criteria = hasContract
	default:
		return []rpc.Transfer{}
	}

	result := make([]rpc.Transfer, 0, len(transfers))

	for _, transfer := range transfers {
		if criteria(transfer) {
			result = append(result, transfer)
		}
	}

	return result
}

// refreshTokenMetadatas will find all of the metadata for an addresses NFTs
func refreshTokenMetadatas(c context.Context, input UpdateTokenInput, tokenRepository persist.TokenRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client) error {
	c = sentryutil.NewSentryHubContext(c)
	c = logger.NewContextWithFields(c, logrus.Fields{"tokenID": input.TokenID, "contractAddress": input.ContractAddress})
	if input.TokenID != "" && input.ContractAddress != "" {
		logger.For(c).Infof("updating metadata for token %s-%s", input.TokenID, input.ContractAddress)
		var token persist.Token

		if input.OwnerAddress != "" {
			var err error
			token, err = tokenRepository.GetByIdentifiers(c, input.TokenID, input.ContractAddress, input.OwnerAddress)
			if err != nil {
				if idErr, ok := err.(persist.ErrTokenNotFoundByIdentifiers); ok {
					logger.For(c).Infof("token not found: %+v", idErr)
					token, err = manuallyIndexToken(c, idErr.TokenID, idErr.ContractAddress, idErr.OwnerAddress, ethClient, tokenRepository)
					if err != nil {
						return err
					}
				} else {
					return err
				}
			}
		} else {
			tokens, err := tokenRepository.GetByTokenIdentifiers(c, input.TokenID, input.ContractAddress, 1, 0)
			if err != nil {
				return err
			}
			token = tokens[0]

		}

		up, err := getUpdateForToken(c, token.TokenType, token.Chain, token.TokenID, token.ContractAddress, token.TokenURI, ethClient, ipfsClient, arweaveClient)
		if err != nil {
			return err
		}
		if err := tokenRepository.UpdateByTokenIdentifiers(c, up.TokenID, up.ContractAddress, up.Update); err != nil {
			return err
		}
		return nil
	}

	var tokens []persist.Token
	var err error
	if input.OwnerAddress != "" {
		tokens, _, err = tokenRepository.GetByWallet(c, input.OwnerAddress, -1, -1)
	} else if input.ContractAddress != "" {
		tokens, err = tokenRepository.GetByContract(c, input.ContractAddress, -1, -1)
	} else {
		return errInvalidUpdateMetadataInput
	}
	if err != nil {
		return err
	}

	tokenUpdateChan := make(chan tokenFullUpdate)
	errChan := make(chan error)
	// iterate over len tokens
	updateMetadataForTokens(c, tokenUpdateChan, errChan, tokens, ethClient, ipfsClient, arweaveClient)
	// == len(tokens) * 2
	for i := 0; i < len(tokens); i++ {
		select {
		case update := <-tokenUpdateChan:
			if err := tokenRepository.UpdateByID(c, update.TokenDBID, update.Update); err != nil {
				logger.For(c).WithError(err).Error("failed to update token in database")
				return err
			}

		case err := <-errChan:
			if err != nil {
				logger.For(c).WithError(err).Error("failed to update metadata for token")
			}
		}
	}
	return nil
}

// updateMetadataForTokens will return two channels that will collectively receive the length of the tokens passed in
func updateMetadataForTokens(pCtx context.Context, updateChan chan<- tokenFullUpdate, errChan chan<- error, tokens []persist.Token, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client) {

	wp := workerpool.New(10)

	for _, t := range tokens {
		token := t
		wp.Submit(func() {

			if t.TokenURI.Type() == persist.URITypeInvalid {
				errChan <- fmt.Errorf("invalid token uri: %s", t.TokenURI)
				return
			}

			up, err := getUpdateForToken(pCtx, token.TokenType, token.Chain, token.TokenID, token.ContractAddress, token.TokenURI, ethClient, ipfsClient, arweaveClient)
			if err != nil {
				errChan <- err
				return
			}

			up.TokenDBID = token.ID

			updateChan <- up

		})
	}
}

func getUpdateForToken(pCtx context.Context, tokenType persist.TokenType, chain persist.Chain, tokenID persist.TokenID, contractAddress persist.EthereumAddress, uri persist.TokenURI, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client) (tokenFullUpdate, error) {

	newURI := uri

	u, err := rpc.GetTokenURI(pCtx, tokenType, persist.EthereumAddress(contractAddress.String()), tokenID, ethClient)
	if err == nil {
		newURI = u.ReplaceID(tokenID)
	} else {
		logEntry := logger.For(pCtx).WithError(err).WithFields(logrus.Fields{
			"tokenType":       tokenType,
			"tokenID":         tokenID,
			"contractAddress": contractAddress,
			"rpcCall":         "eth_call",
		})
		logEthCallRPCError(logEntry, err, fmt.Sprintf("error getting token URI"))
	}

	up := tokenFullUpdate{
		TokenID:         tokenID,
		ContractAddress: persist.EthereumAddress(contractAddress),
		Update: persist.TokenUpdateURIInput{
			TokenURI: newURI,
		},
	}
	return up, nil
}

func updateMetadataFieldsForToken(pCtx context.Context, tokenID persist.TokenID, contractAddress persist.EthereumAddress, tokenMetadata persist.TokenMetadata, tokenRepo persist.TokenRepository) error {
	name, desc := media.FindNameAndDescription(pCtx, tokenMetadata)
	return tokenRepo.UpdateByTokenIdentifiers(pCtx, tokenID, contractAddress, persist.TokenUpdateMetadataFieldsInput{
		Name:        persist.NullString(name),
		Description: persist.NullString(desc),
	})
}

func manuallyIndexToken(pCtx context.Context, tokenID persist.TokenID, contractAddress, ownerAddress persist.EthereumAddress, ec *ethclient.Client, tokenRepo persist.TokenRepository) (t persist.Token, err error) {

	var startingToken persist.Token
	startingTokens, err := tokenRepo.GetByTokenIdentifiers(pCtx, tokenID, contractAddress, 1, 0)
	if err == nil && len(startingTokens) > 0 {
		startingToken = startingTokens[0]
	}

	if handler, ok := customManualIndex[persist.EthereumAddress(contractAddress.String())]; ok {
		handledToken, err := handler(pCtx, tokenID, ownerAddress, ec)
		if err != nil {
			return t, err
		}
		t = handledToken
	} else {

		startingToken.TokenID = tokenID
		startingToken.ContractAddress = contractAddress
		startingToken.OwnerAddress = ownerAddress

		var e721 *contracts.IERC721Caller
		var e1155 *contracts.IERC1155Caller

		e721, err = contracts.NewIERC721Caller(contractAddress.Address(), ec)
		if err != nil {
			return
		}
		e1155, err = contracts.NewIERC1155Caller(contractAddress.Address(), ec)
		if err != nil {
			return
		}
		owner, err := e721.OwnerOf(&bind.CallOpts{Context: pCtx}, tokenID.BigInt())
		isERC721 := err == nil
		if isERC721 {
			startingToken.TokenType = persist.TokenTypeERC721
			startingToken.OwnerAddress = persist.EthereumAddress(owner.String())
		} else {
			bal, err := e1155.BalanceOf(&bind.CallOpts{Context: pCtx}, ownerAddress.Address(), tokenID.BigInt())
			if err != nil {
				return persist.Token{}, fmt.Errorf("failed to get balance or owner for token %s-%s: %s", contractAddress, tokenID, err)
			}
			startingToken.TokenType = persist.TokenTypeERC1155
			startingToken.Quantity = persist.HexString(bal.Text(16))
			startingToken.OwnerAddress = ownerAddress
		}

		t = startingToken
	}
	if err := tokenRepo.Upsert(pCtx, t); err != nil {
		return persist.Token{}, fmt.Errorf("failed to upsert token %s-%s: %s", contractAddress, tokenID, err)
	}

	return t, nil

}
