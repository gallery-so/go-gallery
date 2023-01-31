package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
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
	"github.com/mikeydub/go-gallery/validate"
	"github.com/sirupsen/logrus"
)

type manualIndexHandler func(context.Context, persist.TokenID, persist.EthereumAddress, *ethclient.Client) (persist.Token, error)

var errInvalidUpdateMediaInput = errors.New("must provide either owner_address or token_id and contract_address")

var mediaDownloadLock = &sync.Mutex{}

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
	Update          persist.TokenUpdateAllURIDerivedFieldsInput
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

// GetTokensOutput is the response of the get tokens handler
type GetTokensOutput struct {
	NFTs      []persist.Token    `json:"nfts"`
	Contracts []persist.Contract `json:"contracts"`
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

// UniqueMetadataUpdateErr is returned when an update for an address with a custom handler
// i.e. CryptoPunks, Autoglyphs, etc. fails to update a token.
type UniqueMetadataUpdateErr struct {
	contractAddress persist.Address
	tokenID         persist.TokenID
	err             error
}

func (e UniqueMetadataUpdateErr) Error() string {
	return fmt.Sprintf("failed to get unique metadata for address=%s;token=%s: %s", e.contractAddress, e.tokenID, e.err)
}

// MetadataUpdateErr is returned when an update for an address with a "standard" metadata URI
// i.e. JSON, SVG, IPFS, HTTP, etc. fails to update.
type MetadataUpdateErr struct {
	contractAddress persist.Address
	tokenID         persist.TokenID
	err             error
}

func (e MetadataUpdateErr) Error() string {
	return fmt.Sprintf("failed to get metadata for address=%s;token=%s: %s", e.contractAddress, e.tokenID, e.err)
}

// MetadataPreviewUpdateErr is returned when preview creation failed for a token.
type MetadataPreviewUpdateErr struct {
	contractAddress persist.Address
	tokenID         persist.TokenID
	err             error
}

func (e MetadataPreviewUpdateErr) Error() string {
	return fmt.Sprintf("failed to make media for address=%s;token=%s: %s", e.contractAddress, e.tokenID, e.err)
}

func getTokens(queueChan chan<- processTokensInput, nftRepository persist.TokenRepository, contractRepository persist.ContractRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client, arweaveClient *goar.Client, storageClient *storage.Client) gin.HandlerFunc {
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

type processTokensInput struct {
	key       string
	tokens    []persist.Token
	contracts []persist.Contract
}

func processIncompleteTokens(ctx context.Context, inputs <-chan processTokensInput, nftRepository persist.TokenRepository, contractRepository persist.ContractRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client, arweaveClient *goar.Client, storageClient *storage.Client, tokenBucket string, throttler *throttle.Locker) {
	wp := workerpool.New(10)
	for input := range inputs {
		i := input
		c, cancel := context.WithTimeout(ctx, time.Second*5)
		func() {
			defer cancel()
			err := throttler.Lock(c, i.key)
			if err == nil {
				wp.Submit(func() {
					ctx := sentryutil.NewSentryHubContext(ctx)
					ctx, cancel := context.WithTimeout(ctx, time.Minute*30)
					defer cancel()
					defer throttler.Unlock(ctx, i.key)
					tokensWithoutMedia := make([]persist.Token, 0, len(i.tokens))
					contractsWithoutMedia := make([]persist.Contract, 0, len(i.contracts))
					tokensWithoutMetadataFields := make([]persist.Token, 0, len(i.tokens))
					for _, token := range i.tokens {
						// if the media is not servable, we need to update it as well as the fields it is derived from that could be causing it not to be valid
						// (token URI, metadata, etc.)
						if !token.Media.IsServable() {
							tokensWithoutMedia = append(tokensWithoutMedia, token)
						} else if token.Name == "" || token.Description == "" {
							// token.Media.IsServable() because the tokens in tokensWithoutMedia will have all their metadata fields updated as well, including the ones that wold be updated for tokensWithoutMetadataFields
							// we don't want to update them twice.
							// also, the media being servable guaruntees that the metadata is valid, meaning that we actually have somewhere to look from the metadata fields
							tokensWithoutMetadataFields = append(tokensWithoutMetadataFields, token)
						}
					}
					for _, contract := range i.contracts {
						// the contract name is the most important field and the only field we use at the indexer level as far as contract metadata goes, but we may want to consider updating if other fields are empty in the future as well (such as a description if we start retrieving that field from somewhere)
						if contract.Name == "" {
							contractsWithoutMedia = append(contractsWithoutMedia, contract)
						}
					}

					nwp := workerpool.New(10)
					for _, token := range tokensWithoutMedia {
						t := token
						nwp.Submit(func() {
							ctx := sentryutil.NewSentryHubContext(ctx)
							err := refreshTokenMedias(ctx, UpdateTokenInput{TokenID: t.TokenID, ContractAddress: t.ContractAddress}, nftRepository, ethClient, ipfsClient, arweaveClient, storageClient, tokenBucket)
							if err != nil {
								logEntry := logger.For(ctx).WithError(err).WithFields(logrus.Fields{
									"tokenID":         t.TokenID,
									"contractAddress": t.ContractAddress,
								})

								// Don't report errors for tokens with generic handlers because they fail frequently
								// because of the nature of those URIs.
								var updateErr MetadataUpdateErr
								if errors.As(err, &updateErr) {
									logEntry.Warn("failed to update token media")
								} else {
									logEntry.Error("failed to update token media")
								}
							}
						})
					}
					for _, contract := range contractsWithoutMedia {
						c := contract
						nwp.Submit(func() {
							ctx := sentryutil.NewSentryHubContext(ctx)
							err := updateMediaForContract(ctx, UpdateContractMediaInput{Address: c.Address}, ethClient, contractRepository)
							if err != nil {
								logEntry := logger.For(ctx).WithError(err).WithFields(logrus.Fields{"contractAddress": c.Address})
								logEthCallRPCError(logEntry, err, "failed to update contract media")
							}
						})
					}
					// these tokens have valid metadata and media but are missing metadata fields (name, description)
					for _, token := range tokensWithoutMetadataFields {
						t := token
						nwp.Submit(func() {
							ctx := sentryutil.NewSentryHubContext(ctx)
							err := updateMetadataFieldsForToken(ctx, t.TokenID, t.ContractAddress, t.TokenMetadata, nftRepository)
							if err != nil {
								logEntry := logger.For(ctx).WithError(err).WithFields(logrus.Fields{
									"tokenID":         t.TokenID,
									"contractAddress": t.ContractAddress,
								})
								logEntry.Error("failed to update token metadata fields")
							}
						})
					}
					nwp.StopWait()
					logger.For(ctx).Infof("Successfully processed %d tokens and %d contracts", len(tokensWithoutMedia), len(contractsWithoutMedia))
				})
			} else {
				logger.For(ctx).WithError(err).Warn("failed to acquire lock")
			}
		}()
	}
	wp.StopWait()
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

func validateWalletsNFTs(tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input ValidateWalletNFTsInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		output, err := validateNFTs(c, input, tokenRepository, contractRepository, ethcl, ipfsClient, arweaveClient, stg, tokenBucket)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, output)

	}
}

// validateNFTs will validate the NFTs for the wallet passed in when being compared with opensea
func validateNFTs(c context.Context, input ValidateWalletNFTsInput, tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string) (ValidateUsersNFTsOutput, error) {

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

	openseaAssets, err := opensea.FetchAssetsForWallet(c, input.Wallet, "", 0, nil)
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

		if err := processUnaccountedForNFTs(c, allUnaccountedForAssets, input.Wallet, tokenRepository, contractRepository, ethcl, ipfsClient, arweaveClient, stg, tokenBucket); err != nil {
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
func processUnaccountedForNFTs(ctx context.Context, assets []opensea.Asset, address persist.EthereumAddress, tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string) error {
	errChan := make(chan error)
	wp := workerpool.New(10)
	for _, asset := range assets {
		a := asset
		wp.Submit(func() {
			errChan <- refreshTokenMedias(ctx, UpdateTokenInput{OwnerAddress: address, TokenID: persist.TokenID(a.TokenID.ToBase16()), ContractAddress: a.Contract.ContractAddress, UpdateAll: true}, tokenRepository, ethcl, ipfsClient, arweaveClient, stg, tokenBucket)
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

func updateTokens(tokenRepository persist.TokenRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenBucket string) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := UpdateTokenInput{}
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		err := refreshTokenMedias(c, input, tokenRepository, ethClient, ipfsClient, arweaveClient, storageClient, tokenBucket)
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

// refreshTokenMedias will find all of the media content for an addresses NFTs and possibly cache it in a storage bucket
func refreshTokenMedias(c context.Context, input UpdateTokenInput, tokenRepository persist.TokenRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenBucket string) error {
	c = sentryutil.NewSentryHubContext(c)
	c = logger.NewContextWithFields(c, logrus.Fields{"tokenID": input.TokenID, "contractAddress": input.ContractAddress})
	if input.TokenID != "" && input.ContractAddress != "" {
		logger.For(c).Infof("updating media for token %s-%s", input.TokenID, input.ContractAddress)
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

		up, err := getUpdateForToken(c, uniqueMetadataHandlers, token.TokenType, token.Chain, token.TokenID, token.ContractAddress, token.TokenMetadata, token.TokenURI, token.Media.MediaType, ethClient, ipfsClient, arweaveClient, storageClient, tokenBucket)
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
		return errInvalidUpdateMediaInput
	}
	if err != nil {
		return err
	}

	if !input.UpdateAll {
		res := make([]persist.Token, 0, len(tokens))
		for _, token := range tokens {
			switch token.Media.MediaType {
			case persist.MediaTypeVideo:
				if token.Media.MediaURL == "" || token.Media.ThumbnailURL == "" {
					res = append(res, token)
				}
			default:
				if token.Media.MediaURL == "" || token.Media.MediaType == "" {
					res = append(res, token)
				}
			}
		}
		tokens = res
	}

	tokenUpdateChan := make(chan tokenFullUpdate)
	errChan := make(chan error)
	// iterate over len tokens
	updateMediaForTokens(c, tokenUpdateChan, errChan, tokens, ethClient, ipfsClient, arweaveClient, storageClient, tokenBucket)
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
				logger.For(c).WithError(err).Error("failed to update media for token")
			}
		}
	}
	return nil
}

// updateMediaForTokens will return two channels that will collectively receive the length of the tokens passed in
func updateMediaForTokens(pCtx context.Context, updateChan chan<- tokenFullUpdate, errChan chan<- error, tokens []persist.Token, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenBucket string) {

	wp := workerpool.New(10)

	for _, t := range tokens {
		token := t
		wp.Submit(func() {

			if t.TokenURI.Type() == persist.URITypeInvalid {
				errChan <- fmt.Errorf("invalid token uri: %s", t.TokenURI)
				return
			}

			up, err := getUpdateForToken(pCtx, uniqueMetadataHandlers, token.TokenType, token.Chain, token.TokenID, token.ContractAddress, token.TokenMetadata, token.TokenURI, token.Media.MediaType, ethClient, ipfsClient, arweaveClient, storageClient, tokenBucket)
			if err != nil {
				errChan <- err
				return
			}

			up.TokenDBID = token.ID

			updateChan <- up

		})
	}
}

func getUpdateForToken(pCtx context.Context, uniqueHandlers uniqueMetadatas, tokenType persist.TokenType, chain persist.Chain, tokenID persist.TokenID, contractAddress persist.EthereumAddress, metadata persist.TokenMetadata, uri persist.TokenURI, mediaType persist.MediaType, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenBucket string) (tokenFullUpdate, error) {
	newMetadata := metadata
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

	if handler, ok := uniqueHandlers[persist.EthereumAddress(contractAddress.String())]; ok {
		logger.For(pCtx).Infof("Using %v metadata handler for %s", handler, contractAddress)
		u, md, err := handler(pCtx, newURI, persist.EthereumAddress(contractAddress.String()), tokenID, ethClient, ipfsClient, arweaveClient)
		if err != nil {
			return tokenFullUpdate{}, UniqueMetadataUpdateErr{
				contractAddress: persist.Address(contractAddress),
				tokenID:         tokenID,
				err:             err,
			}
		}
		newMetadata = md
		newURI = u
	} else {
		md, err := rpc.GetMetadataFromURI(pCtx, newURI, ipfsClient, arweaveClient)
		if err != nil {
			return tokenFullUpdate{}, MetadataUpdateErr{
				contractAddress: persist.Address(contractAddress),
				tokenID:         tokenID,
				err:             err,
			}
		}
		newMetadata = md
	}

	name, description := media.FindNameAndDescription(pCtx, newMetadata)

	image, animation := media.KeywordsForChain(chain, imageKeywords, animationKeywords)

	newMedia, err := media.MakePreviewsForMetadata(pCtx, newMetadata, persist.Address(contractAddress.String()), tokenID, newURI, chain, ipfsClient, arweaveClient, storageClient, tokenBucket, image, animation)
	if err != nil {
		return tokenFullUpdate{}, MetadataPreviewUpdateErr{
			contractAddress: persist.Address(contractAddress),
			tokenID:         tokenID,
			err:             err,
		}
	}
	up := tokenFullUpdate{
		TokenID:         tokenID,
		ContractAddress: persist.EthereumAddress(contractAddress),
		Update: persist.TokenUpdateAllURIDerivedFieldsInput{
			TokenURI:    newURI,
			Metadata:    newMetadata,
			Media:       newMedia,
			Name:        persist.NullString(validate.SanitizationPolicy.Sanitize(name)),
			Description: persist.NullString(validate.SanitizationPolicy.Sanitize(description)),
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
