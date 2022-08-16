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
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
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
			TokenType:       persist.TokenTypeERC721,
			OwnerAddress:    persist.EthereumAddress(owner.String()),
			ContractAddress: persist.EthereumAddress("0xb47e3cd837ddf8e4c57f05d70ab865de6e193bbb"),
			TokenID:         ti,
		}, nil
	},
}

// UpdateTokenMediaInput is the input for the update media endpoint that will find all of the media content
// for an addresses NFTs and cache it in a storage bucket
type UpdateTokenMediaInput struct {
	OwnerAddress    persist.EthereumAddress `json:"owner_address,omitempty"`
	TokenID         persist.TokenID         `json:"token_id,omitempty"`
	ContractAddress persist.EthereumAddress `json:"contract_address,omitempty"`
	UpdateAll       bool                    `json:"update_all"`
}

type tokenUpdate struct {
	TokenDBID       persist.DBID
	TokenID         persist.TokenID
	ContractAddress persist.EthereumAddress
	Update          persist.TokenUpdateMediaInput
}

type getTokensInput struct {
	WalletAddress   persist.EthereumAddress `form:"address"`
	ContractAddress persist.EthereumAddress `form:"contract_address"`
	TokenID         persist.TokenID         `form:"token_id"`
	Page            int64                   `form:"page"`
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

func processMedialessTokens(ctx context.Context, inputs <-chan processTokensInput, nftRepository persist.TokenRepository, contractRepository persist.ContractRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client, arweaveClient *goar.Client, storageClient *storage.Client, tokenBucket string, throttler *throttle.Locker) {
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
					for _, token := range i.tokens {
						if token.Media.MediaURL == "" || token.Media.MediaType == "" || token.Media.MediaType == persist.MediaTypeUnknown {
							tokensWithoutMedia = append(tokensWithoutMedia, token)
						}
					}
					for _, contract := range i.contracts {
						if contract.Name == "" {
							contractsWithoutMedia = append(contractsWithoutMedia, contract)
						}
					}

					nwp := workerpool.New(10)
					for _, token := range tokensWithoutMedia {
						t := token
						nwp.Submit(func() {
							ctx := sentryutil.NewSentryHubContext(ctx)
							err := refreshToken(ctx, UpdateTokenMediaInput{TokenID: t.TokenID, ContractAddress: t.ContractAddress}, nftRepository, ethClient, ipfsClient, arweaveClient, storageClient, tokenBucket)
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
		return tokenRepo.GetByWallet(pCtx, input.WalletAddress, input.Limit, input.Page)
	case input.TokenID != "" && input.ContractAddress != "":
		if strings.HasPrefix(string(input.TokenID), "0x") {
			input.TokenID = input.TokenID[2:]
		} else {
			input.TokenID = persist.TokenID(input.TokenID.BigInt().Text(16))
		}

		tokens, err := tokenRepo.GetByTokenIdentifiers(pCtx, input.TokenID, input.ContractAddress, input.Limit, input.Page)
		if err != nil {
			return nil, nil, err
		}
		contract, err := contractRepo.GetByAddress(pCtx, input.ContractAddress)
		if err != nil {
			return nil, nil, err
		}
		return tokens, []persist.Contract{contract}, nil
	case input.ContractAddress != "":
		tokens, err := tokenRepo.GetByContract(pCtx, input.ContractAddress, input.Limit, input.Page)
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

func validateWalletsNFTs(tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input ValidateWalletNFTsInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		output, err := validateNFTs(c, input, tokenRepository, contractRepository, ethcl, ipfsClient, arweaveClient, stg)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
		}
		c.JSON(http.StatusOK, output)

	}
}

// validateNFTs will validate the NFTs for the wallet passed in when being compared with opensea
func validateNFTs(c context.Context, input ValidateWalletNFTsInput, tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) (ValidateUsersNFTsOutput, error) {

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

		if err := processUnaccountedForNFTs(c, allUnaccountedForAssets, input.Wallet, tokenRepository, contractRepository, ethcl, ipfsClient, arweaveClient, stg); err != nil {
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
			update := persist.TokenUpdateMediaInput{
				TokenURI: token.TokenURI,
				Metadata: token.TokenMetadata,
				Media:    token.Media,
			}

			if err := tokenRepository.UpdateByID(ctx, token.ID, update); err != nil {
				return "", fmt.Errorf("failed to update token %s: %v", token.TokenID, err)
			}
		}

	}
	return msgToAdd, nil
}
func processUnaccountedForNFTs(ctx context.Context, assets []opensea.Asset, address persist.EthereumAddress, tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) error {
	allTokens := make([]persist.Token, 0, len(assets))
	cntracts := make([]persist.Contract, 0, len(assets))
	block, err := ethcl.BlockNumber(ctx)
	if err != nil {
		return err
	}
	for _, a := range assets {

		logger.For(ctx).Debugf("processing asset: %+v", a)

		asURI := persist.TokenURI(a.ImageURL)
		media := persist.Media{}

		bs, err := rpc.GetDataFromURI(ctx, asURI, ipfsClient, arweaveClient)
		if err == nil {
			mediaType := persist.SniffMediaType(bs)
			if mediaType != persist.MediaTypeUnknown {
				media = persist.Media{
					MediaURL:     persist.NullString(a.ImageURL),
					ThumbnailURL: persist.NullString(a.ImagePreviewURL),
					MediaType:    mediaType,
				}
			}
		}

		logger.For(ctx).Debugf("media: %+v", media)

		metadata, _ := rpc.GetMetadataFromURI(ctx, persist.TokenURI(a.TokenMetadataURL).ReplaceID(persist.TokenID(a.TokenID.ToBase16())), ipfsClient, arweaveClient)

		logger.For(ctx).Debugf("metadata: %+v", metadata)
		t := persist.Token{
			Name:            persist.NullString(validate.SanitizationPolicy.Sanitize(a.Name)),
			Description:     persist.NullString(validate.SanitizationPolicy.Sanitize(a.Description)),
			Chain:           persist.ChainETH,
			TokenID:         persist.TokenID(a.TokenID.ToBase16()),
			ContractAddress: a.Contract.ContractAddress,
			OwnerAddress:    a.Owner.Address,
			TokenURI:        persist.TokenURI(a.TokenMetadataURL),
			ExternalURL:     persist.NullString(a.ExternalURL),
			TokenMetadata:   metadata,
			Media:           media,
			Quantity:        "1",
			BlockNumber:     persist.BlockNumber(block),
			OwnershipHistory: []persist.EthereumAddressAtBlock{
				{
					Address: persist.ZeroAddress,
					Block:   persist.BlockNumber(block - 1),
				},
			},
		}
		switch a.Contract.ContractSchemaName {
		case "ERC721", "CRYPTOPUNKS":
			t.TokenType = persist.TokenTypeERC721
			allTokens = append(allTokens, t)
		case "ERC1155":
			t.TokenType = persist.TokenTypeERC1155
			ierc1155, err := contracts.NewIERC1155Caller(t.ContractAddress.Address(), ethcl)
			if err != nil {
				return err
			}

			new := t
			bal, err := ierc1155.BalanceOf(&bind.CallOpts{Context: ctx}, address.Address(), t.TokenID.BigInt())
			if err != nil {
				return err
			}
			if bal.Cmp(bigZero) > 0 {
				new.OwnerAddress = address
				new.Quantity = persist.HexString(bal.Text(16))

				allTokens = append(allTokens, new)
			}

		default:
			return fmt.Errorf("unsupported token type: %s", a.Contract.ContractSchemaName)
		}

		c := persist.Contract{
			Address:     a.Contract.ContractAddress,
			Symbol:      a.Contract.ContractSymbol,
			Name:        a.Contract.ContractName,
			LatestBlock: persist.BlockNumber(block),
		}
		cntracts = append(cntracts, c)
	}

	if err := contractRepository.BulkUpsert(ctx, cntracts); err != nil {
		return err
	}

	logger.For(ctx).Infof("found %d new tokens", len(allTokens))

	if err := tokenRepository.BulkUpsert(ctx, allTokens); err != nil {
		return err
	}

	return nil
}

func updateTokens(tokenRepository persist.TokenRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenBucket string) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := UpdateTokenMediaInput{}
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		err := refreshToken(c, input, tokenRepository, ethClient, ipfsClient, arweaveClient, storageClient, tokenBucket)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})

	}
}

// refreshToken will find all of the media content for an addresses NFTs and possibly cache it in a storage bucket
func refreshToken(c context.Context, input UpdateTokenMediaInput, tokenRepository persist.TokenRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenBucket string) error {
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

	tokenUpdateChan := make(chan tokenUpdate)
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
func updateMediaForTokens(pCtx context.Context, updateChan chan<- tokenUpdate, errChan chan<- error, tokens []persist.Token, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenBucket string) {

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

func getUpdateForToken(pCtx context.Context, uniqueHandlers uniqueMetadatas, tokenType persist.TokenType, chain persist.Chain, tokenID persist.TokenID, contractAddress persist.EthereumAddress, metadata persist.TokenMetadata, uri persist.TokenURI, mediaType persist.MediaType, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenBucket string) (tokenUpdate, error) {
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
			return tokenUpdate{}, UniqueMetadataUpdateErr{
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
			return tokenUpdate{}, MetadataUpdateErr{
				contractAddress: persist.Address(contractAddress),
				tokenID:         tokenID,
				err:             err,
			}
		}
		newMetadata = md
	}

	name, ok := util.GetValueFromMap(newMetadata, "name", util.DefaultSearchDepth).(string)
	if !ok {
		name = ""
	}
	description, ok := util.GetValueFromMap(newMetadata, "description", util.DefaultSearchDepth).(string)
	if !ok {
		description = ""
	}

	newMedia, err := media.MakePreviewsForMetadata(pCtx, newMetadata, contractAddress.String(), tokenID, newURI, chain, ipfsClient, arweaveClient, storageClient, tokenBucket, imageKeywords, animationKeywords)
	if err != nil {
		return tokenUpdate{}, MetadataPreviewUpdateErr{
			contractAddress: persist.Address(contractAddress),
			tokenID:         tokenID,
			err:             err,
		}
	}
	up := tokenUpdate{
		TokenID:         tokenID,
		ContractAddress: persist.EthereumAddress(contractAddress),
		Update: persist.TokenUpdateMediaInput{
			TokenURI:    newURI,
			Metadata:    newMetadata,
			Media:       newMedia,
			Name:        persist.NullString(validate.SanitizationPolicy.Sanitize(name)),
			Description: persist.NullString(validate.SanitizationPolicy.Sanitize(description)),
		},
	}
	return up, nil
}

func manuallyIndexToken(pCtx context.Context, tokenID persist.TokenID, contractAddress, ownerAddress persist.EthereumAddress, ec *ethclient.Client, tokenRepo persist.TokenRepository) (t persist.Token, err error) {

	if handler, ok := customManualIndex[persist.EthereumAddress(contractAddress.String())]; ok {
		handledToken, err := handler(pCtx, tokenID, ownerAddress, ec)
		if err != nil {
			return t, err
		}
		t = handledToken
	} else {

		t.TokenID = tokenID
		t.ContractAddress = contractAddress

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
			t.TokenType = persist.TokenTypeERC721
			t.OwnerAddress = persist.EthereumAddress(owner.String())
		} else {
			bal, err := e1155.BalanceOf(&bind.CallOpts{Context: pCtx}, ownerAddress.Address(), tokenID.BigInt())
			if err != nil {
				return persist.Token{}, fmt.Errorf("failed to get balance or owner for token %s-%s: %s", contractAddress, tokenID, err)
			}
			t.TokenType = persist.TokenTypeERC1155
			t.Quantity = persist.HexString(bal.Text(16))
			t.OwnerAddress = ownerAddress
		}
	}
	if err := tokenRepo.Upsert(pCtx, t); err != nil {
		return persist.Token{}, fmt.Errorf("failed to upsert token %s-%s: %s", contractAddress, tokenID, err)
	}

	return t, nil

}
