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
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gammazero/workerpool"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
	"github.com/sirupsen/logrus"
)

var errInvalidUpdateMediaInput = errors.New("must provide either owner_address or token_id and contract_address")

var mediaDownloadLock = &sync.Mutex{}

var bigZero = big.NewInt(0)

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
	Update          interface{}
}

type getTokensInput struct {
	ID              persist.DBID            `form:"id"`
	WalletAddress   persist.EthereumAddress `form:"address"`
	ContractAddress persist.EthereumAddress `form:"contract_address"`
	TokenID         persist.TokenID         `form:"token_id"`
	Page            int64                   `form:"page"`
	Limit           int64                   `form:"limit"`
}

// GetTokenOutput is the response of the get token handler
type GetTokenOutput struct {
	NFT      persist.Token    `json:"nft"`
	Contract persist.Contract `json:"contract"`
}

// GetTokensOutput is the response of the get tokens handler
type GetTokensOutput struct {
	NFTs      []persist.Token    `json:"nfts"`
	Contracts []persist.Contract `json:"contracts"`
}

// ValidateUsersNFTsInput is the input for the validate users NFTs endpoint that will return
// whether what opensea has on a user is the same as what we have in our database
type ValidateUsersNFTsInput struct {
	Wallet persist.EthereumAddress `json:"wallet,omitempty" binding:"required"`
	All    bool                    `json:"all"`
}

// ValidateUsersNFTsOutput is the output of the validate users NFTs endpoint that will return
// whether what opensea has on a user is the same as what we have in our database
type ValidateUsersNFTsOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

func getTokens(queueChan chan<- processTokensInput, nftRepository persist.TokenRepository, contractRepository persist.ContractRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client, arweaveClient *goar.Client, storageClient *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getTokensInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		if input.ID == "" && input.WalletAddress == "" && input.ContractAddress == "" && input.TokenID == "" {
			util.ErrResponse(c, http.StatusBadRequest, util.ErrInvalidInput{Reason: "must specify at least one of id, address, contract_address, token_id"})
			return
		}

		token, contract, err := getTokenFromDB(c, input, nftRepository, contractRepository)
		if err != nil {
			status := http.StatusInternalServerError
			if _, ok := err.(persist.ErrTokenNotFoundByID); ok {
				status = http.StatusNotFound
			}
			util.ErrResponse(c, status, err)
			return
		}

		if token.ID != "" {
			c.JSON(http.StatusOK, GetTokenOutput{NFT: token, Contract: contract})
			return
		}
		tokens, contracts, err := getTokensFromDB(c, input, nftRepository, contractRepository)
		if err != nil {
			status := http.StatusInternalServerError
			if _, ok := err.(persist.ErrTokenNotFoundByIdentifiers); ok {
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

func processMedialessTokens(inputs <-chan processTokensInput, nftRepository persist.TokenRepository, contractRepository persist.ContractRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client, arweaveClient *goar.Client, storageClient *storage.Client, throttler *throttle.Locker) {
	wp := workerpool.New(10)
	for input := range inputs {
		i := input
		c, cancel := context.WithTimeout(context.Background(), time.Second*5)
		func() {
			defer cancel()
			err := throttler.Lock(c, i.key)
			if err == nil {
				wp.Submit(func() {
					ctx, cancel := context.WithTimeout(context.Background(), time.Minute*30)
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
							err := refreshToken(ctx, UpdateTokenMediaInput{TokenID: t.TokenID, ContractAddress: t.ContractAddress}, nftRepository, ethClient, ipfsClient, arweaveClient, storageClient)
							if err != nil {
								logrus.Errorf("failed to update token media: %s", err)
							}
						})
					}
					for _, contract := range contractsWithoutMedia {
						c := contract
						nwp.Submit(func() {
							err := updateMediaForContract(ctx, UpdateContractMediaInput{Address: c.Address}, ethClient, contractRepository)
							if err != nil {
								logrus.Errorf("failed to update contract media: %s", err)
							}
						})
					}
					nwp.StopWait()
				})
			} else {
				logrus.Errorf("failed to acquire lock: %s", err)
			}
		}()
	}
	wp.StopWait()
}

func getTokenFromDB(pCtx context.Context, input *getTokensInput, tokenRepo persist.TokenRepository, contractRepo persist.ContractRepository) (persist.Token, persist.Contract, error) {
	switch {
	case input.ID != "":
		token, err := tokenRepo.GetByID(pCtx, input.ID)
		if err != nil {
			return persist.Token{}, persist.Contract{}, nil
		}
		contract, err := contractRepo.GetByAddress(pCtx, token.ContractAddress)
		if err != nil {
			return persist.Token{}, persist.Contract{}, nil
		}
		return token, contract, nil
	}
	return persist.Token{}, persist.Contract{}, nil
}
func getTokensFromDB(pCtx context.Context, input *getTokensInput, tokenRepo persist.TokenRepository, contractRepo persist.ContractRepository) ([]persist.Token, []persist.Contract, error) {
	switch {
	case input.ID != "":
		token, err := tokenRepo.GetByID(pCtx, input.ID)
		if err != nil {
			return nil, nil, err
		}
		contract, err := contractRepo.GetByAddress(pCtx, token.ContractAddress)
		if err != nil {
			return nil, nil, err
		}
		return []persist.Token{token}, []persist.Contract{contract}, nil
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
		var input ValidateUsersNFTsInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		// TODO do we need to validate at the indexer level? probably multichain level.
		// output, err := ValidateNFTs(c, input, tokenRepository, contractRepository, ethcl, ipfsClient, arweaveClient, stg)
		// if err != nil {
		// 	util.ErrResponse(c, http.StatusInternalServerError, err)
		// }
		// c.JSON(http.StatusOK, output)

	}
}

// validateNFTs will validate the NFTs for the wallet passed in when being compared with opensea
func validateNFTs(c context.Context, input ValidateUsersNFTsInput, userRepository persist.UserRepository, tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) (ValidateUsersNFTsOutput, error) {

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

		if err := processUnaccountedForNFTs(c, allUnaccountedForAssets, input.Wallet, tokenRepository, contractRepository, userRepository, ethcl, ipfsClient, arweaveClient, stg); err != nil {
			logrus.WithError(err).Error("failed to process unaccounted for NFTs")
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
			logrus.Errorf("failed to get token URI for token %s-%s: %v", token.ContractAddress, token.TokenID, err)
			msgToAdd += fmt.Sprintf("failed to get token URI for token %s-%s: %v\n", token.ContractAddress, token.TokenID, err)
			continue
		}

		metadata, err := rpc.GetMetadataFromURI(ctx, token.TokenURI, ipfsClient, arweaveClient)
		if err == nil {
			token.TokenMetadata = metadata
			needsUpdate = true
		} else {
			logrus.Errorf("failed to get token metadata for token %s-%s with uri %s: %v", token.ContractAddress, token.TokenID, token.TokenURI, err)
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
func processUnaccountedForNFTs(ctx context.Context, assets []opensea.Asset, address persist.EthereumAddress, tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, userRepository persist.UserRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) error {
	allTokens := make([]persist.Token, 0, len(assets))
	cntracts := make([]persist.Contract, 0, len(assets))
	block, err := ethcl.BlockNumber(ctx)
	if err != nil {
		return err
	}
	for _, a := range assets {

		logrus.Debugf("processing asset: %+v", a)

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

		logrus.Debugf("media: %+v", media)

		metadata, _ := rpc.GetMetadataFromURI(ctx, persist.TokenURI(a.TokenMetadataURL).ReplaceID(persist.TokenID(a.TokenID.ToBase16())), ipfsClient, arweaveClient)

		logrus.Debugf("metadata: %+v", metadata)
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

	logrus.Infof("found %d new tokens", len(allTokens))

	if err := tokenRepository.BulkUpsert(ctx, allTokens); err != nil {
		return err
	}

	return nil
}

func updateTokens(tokenRepository persist.TokenRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := UpdateTokenMediaInput{}
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		err := refreshToken(c, input, tokenRepository, ethClient, ipfsClient, arweaveClient, storageClient)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})

	}
}

// refreshToken will find all of the media content for an addresses NFTs and possibly cache it in a storage bucket
func refreshToken(c context.Context, input UpdateTokenMediaInput, tokenRepository persist.TokenRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) error {
	if input.TokenID != "" && input.ContractAddress != "" {
		logrus.Infof("updating media for token %s-%s", input.TokenID, input.ContractAddress)
		tokens, err := tokenRepository.GetByTokenIdentifiers(c, input.TokenID, input.ContractAddress, 1, 0)
		if err != nil {
			return err
		}
		token := tokens[0]

		newOwner, newQuantity, newBlock, err := getOwnershipUpdate(c, token, ethClient)
		if err != nil {
			return err
		}
		err = updateOwnershipStatus(c, tokenRepository, token.ID, token.TokenType, newOwner, token.OwnerAddress, newQuantity, token.Quantity, newBlock)
		if err != nil {
			return err
		}

		up, err := getUpdateForToken(c, uniqueMetadataHandlers, token.TokenType, token.Chain, token.TokenID, token.ContractAddress, token.TokenMetadata, token.TokenURI, token.Media.MediaType, ethClient, ipfsClient, arweaveClient, storageClient)
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
	updateMediaForTokens(c, tokenUpdateChan, errChan, tokens, ethClient, ipfsClient, arweaveClient, storageClient)
	// iterate again also len tokens
	updateOwnershipForTokens(c, tokenUpdateChan, errChan, tokens, ethClient)
	// == len(tokens) * 2
	for i := 0; i < len(tokens)*2; i++ {
		select {
		case update := <-tokenUpdateChan:
			if err := tokenRepository.UpdateByID(c, update.TokenDBID, update.Update); err != nil {
				logrus.WithError(err).Error("failed to update token in database")
				if perr, ok := err.(*pq.Error); ok {
					// if the error is a violation of unique constraint we should delete the token
					if perr.Code == "23505" {
						return tokenRepository.DeleteByID(c, update.TokenDBID)
					}
				}
				return err
			}

		case err := <-errChan:
			if err != nil {
				logrus.WithError(err).Error("failed to update media for token")
			}
		}
	}
	return nil
}

func updateOwnershipStatus(ctx context.Context, tokenRepository persist.TokenRepository, tokenDBID persist.DBID, tokenType persist.TokenType, curOwner, supposedOwner persist.EthereumAddress, curBalance, supposedBalance persist.HexString, block persist.BlockNumber) error {
	switch tokenType {
	case persist.TokenTypeERC1155:
		if strings.EqualFold(curBalance.String(), supposedBalance.String()) {
			return nil
		}
		logger.For(ctx).Infof("balance status for token %s is incorrect: real %s -  before %s", tokenDBID, curBalance, supposedBalance)
		return tokenRepository.UpdateByID(ctx, tokenDBID, persist.TokenUpdateBalanceInput{Quantity: curBalance, BlockNumber: block})
	case persist.TokenTypeERC721:
		if strings.EqualFold(curOwner.String(), supposedOwner.String()) {
			return nil
		}
		logger.For(ctx).Infof("ownership status for token %s is incorrect: real %s -  before %s", tokenDBID, curOwner, supposedOwner)
		err := tokenRepository.UpdateByID(ctx, tokenDBID, persist.TokenUpdateOwnerInput{OwnerAddress: curOwner, BlockNumber: block})
		if err != nil {
			if perr, ok := err.(*pq.Error); ok {
				// if the error is a violation of unique constraint we should delete the token
				if perr.Code == "23505" {
					return tokenRepository.DeleteByID(ctx, tokenDBID)
				}
			}
			return err
		}
		return nil
	default:
		return fmt.Errorf("unsupported token type: %s", tokenType)
	}
}

func getOwnershipUpdate(ctx context.Context, token persist.Token, ethClient *ethclient.Client) (persist.EthereumAddress, persist.HexString, persist.BlockNumber, error) {
	block, err := ethClient.BlockNumber(ctx)
	if err != nil {
		return "", "", 0, err
	}
	switch token.TokenType {
	case persist.TokenTypeERC721:
		erc721, err := contracts.NewIERC721Caller(token.ContractAddress.Address(), ethClient)
		if err != nil {
			return "", "", 0, err
		}
		owner, err := erc721.OwnerOf(&bind.CallOpts{Context: ctx}, token.TokenID.BigInt())
		if err != nil {
			return "", "", 0, err
		}

		return persist.EthereumAddress(owner.String()), token.Quantity, persist.BlockNumber(block), nil
	case persist.TokenTypeERC1155:
		if token.OwnerAddress == "" {
			return token.OwnerAddress, token.Quantity, persist.BlockNumber(block), nil
		}
		erc1155, err := contracts.NewIERC1155Caller(token.ContractAddress.Address(), ethClient)
		if err != nil {
			return "", "", 0, err
		}
		bal, err := erc1155.BalanceOf(&bind.CallOpts{Context: ctx}, token.OwnerAddress.Address(), token.TokenID.BigInt())
		if err != nil {
			return "", "", 0, err
		}
		return token.OwnerAddress, persist.HexString(bal.Text(16)), persist.BlockNumber(block), nil
	default:
		return "", "", 0, fmt.Errorf("unsupported token type %s", token.TokenType)
	}
}

// updateMediaForTokens will return two channels that will collectively receive the length of the tokens passed in
func updateMediaForTokens(pCtx context.Context, updateChan chan<- tokenUpdate, errChan chan<- error, tokens []persist.Token, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) {

	wp := workerpool.New(10)

	for _, t := range tokens {
		token := t
		wp.Submit(func() {

			up, err := getUpdateForToken(pCtx, uniqueMetadataHandlers, token.TokenType, token.Chain, token.TokenID, token.ContractAddress, token.TokenMetadata, token.TokenURI, token.Media.MediaType, ethClient, ipfsClient, arweaveClient, storageClient)
			if err != nil {
				errChan <- err
				return
			}

			up.TokenDBID = token.ID

			updateChan <- up

		})
	}
}

func updateOwnershipForTokens(c context.Context, tokenUpdateChan chan<- tokenUpdate, errChan chan error, tokens []persist.Token, ethClient *ethclient.Client) {
	updateChan := make(chan tokenUpdate)
	wp := workerpool.New(10)

	for _, t := range tokens {
		token := t
		wp.Submit(func() {

			owner, balance, block, err := getOwnershipUpdate(c, token, ethClient)
			if err != nil {
				errChan <- err
				return
			}

			switch token.TokenType {
			case persist.TokenTypeERC1155:
				if strings.EqualFold(token.Quantity.String(), balance.String()) {
					errChan <- nil
					return
				}
				updateChan <- tokenUpdate{TokenDBID: token.ID, Update: persist.TokenUpdateBalanceInput{
					Quantity:    balance,
					BlockNumber: block,
				}}
			case persist.TokenTypeERC721:
				if strings.EqualFold(token.OwnerAddress.String(), owner.String()) {
					errChan <- nil
					return
				}
				updateChan <- tokenUpdate{TokenDBID: token.ID, Update: persist.TokenUpdateOwnerInput{
					OwnerAddress: owner,
					BlockNumber:  block,
				}}
			default:
				errChan <- fmt.Errorf("unsupported token type %s", token.TokenType)
			}
		})
	}
}

func getUpdateForToken(pCtx context.Context, uniqueHandlers uniqueMetadatas, tokenType persist.TokenType, chain persist.Chain, tokenID persist.TokenID, contractAddress persist.EthereumAddress, metadata persist.TokenMetadata, uri persist.TokenURI, mediaType persist.MediaType, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) (tokenUpdate, error) {
	newMetadata := metadata
	newURI := uri

	u, err := rpc.GetTokenURI(pCtx, tokenType, persist.EthereumAddress(contractAddress.String()), tokenID, ethClient)
	if err == nil {
		newURI = u.ReplaceID(tokenID)
	} else {
		logger.For(pCtx).Errorf("error getting token URI: %s", err)
	}

	if handler, ok := uniqueHandlers[persist.EthereumAddress(contractAddress.String())]; ok {
		logrus.Infof("Using %v metadata handler for %s", handler, contractAddress)
		u, md, err := handler(newURI, persist.EthereumAddress(contractAddress.String()), tokenID)
		if err != nil {
			return tokenUpdate{}, fmt.Errorf("failed to get unique metadata for token %s: %s", uri, err)
		}
		newMetadata = md
		newURI = u
	} else {
		md, err := rpc.GetMetadataFromURI(pCtx, newURI, ipfsClient, arweaveClient)
		if err != nil {
			return tokenUpdate{}, fmt.Errorf("failed to get metadata for token %s: %v", tokenID, err)
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

	newMedia, err := media.MakePreviewsForMetadata(pCtx, newMetadata, contractAddress.String(), tokenID, newURI, chain, ipfsClient, arweaveClient, storageClient)
	if err != nil {
		return tokenUpdate{}, fmt.Errorf("failed to make media for token %s-%s: %v", contractAddress, tokenID, err)
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
