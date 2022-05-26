package indexer

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gammazero/workerpool"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

var errInvalidUpdateMediaInput = errors.New("must provide either owner_address or token_id and contract_address")

var mediaDownloadLock = &sync.Mutex{}

var bigZero = big.NewInt(0)

// UpdateMediaInput is the input for the update media endpoint that will find all of the media content
// for an addresses NFTs and cache it in a storage bucket
type UpdateMediaInput struct {
	OwnerAddress    persist.EthereumAddress `json:"owner_address,omitempty"`
	TokenID         persist.TokenID         `json:"token_id,omitempty"`
	ContractAddress persist.EthereumAddress `json:"contract_address,omitempty"`
	UpdateAll       bool                    `json:"update_all"`
}

type tokenUpdateMedia struct {
	TokenDBID       persist.DBID
	TokenID         persist.TokenID
	ContractAddress persist.EthereumAddress
	Update          persist.TokenUpdateMediaInput
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
	NFT persist.Token `json:"nft"`
}

// GetTokensOutput is the response of the get tokens handler
type GetTokensOutput struct {
	NFTs []persist.Token `json:"nfts"`
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

func getTokens(nftRepository persist.TokenRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client) gin.HandlerFunc {
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

		token, err := getTokenFromDB(c, input, nftRepository)
		if err != nil {
			status := http.StatusInternalServerError
			if _, ok := err.(persist.ErrTokenNotFoundByID); ok {
				status = http.StatusNotFound
			}
			util.ErrResponse(c, status, err)
			return
		}

		if token.ID != "" {
			c.JSON(http.StatusOK, GetTokenOutput{NFT: token})
			return
		}
		tokens, err := getTokensFromDB(c, input, nftRepository)
		if err != nil {
			status := http.StatusInternalServerError
			if _, ok := err.(persist.ErrTokenNotFoundByIdentifiers); ok {
				status = http.StatusNotFound
			}
			util.ErrResponse(c, status, err)
			return
		}
		if tokens != nil {

			c.JSON(http.StatusOK, GetTokensOutput{NFTs: tokens})
			return
		}

		util.ErrResponse(c, http.StatusInternalServerError, fmt.Errorf("no tokens found"))
	}
}

func getTokenFromDB(pCtx context.Context, input *getTokensInput, tokenRepo persist.TokenRepository) (persist.Token, error) {
	switch {
	case input.ID != "":
		return tokenRepo.GetByID(pCtx, input.ID)
	}
	return persist.Token{}, nil
}
func getTokensFromDB(pCtx context.Context, input *getTokensInput, tokenRepo persist.TokenRepository) ([]persist.Token, error) {
	switch {
	case input.ID != "":
		token, err := tokenRepo.GetByID(pCtx, input.ID)
		if err != nil {
			return nil, err
		}
		return []persist.Token{token}, nil
	case input.WalletAddress != "":
		return tokenRepo.GetByWallet(pCtx, input.WalletAddress, input.Limit, input.Page)
	case input.TokenID != "" && input.ContractAddress != "":
		if strings.HasPrefix(string(input.TokenID), "0x") {
			input.TokenID = input.TokenID[2:]
		} else {
			input.TokenID = persist.TokenID(input.TokenID.BigInt().Text(16))
		}

		return tokenRepo.GetByTokenIdentifiers(pCtx, input.TokenID, input.ContractAddress, input.Limit, input.Page)
	case input.ContractAddress != "":
		return tokenRepo.GetByContract(pCtx, input.ContractAddress, input.Limit, input.Page)
	}
	return nil, nil

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

// ValidateNFTs will validate the NFTs for the wallet passed in when being compared with opensea
func ValidateNFTs(c context.Context, input ValidateUsersNFTsInput, userRepository persist.UserRepository, tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) (ValidateUsersNFTsOutput, error) {

	currentNFTs, err := tokenRepository.GetByWallet(c, input.Wallet, -1, 0)
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
			Name:            persist.NullString(a.Name),
			Description:     persist.NullString(a.Description),
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

func updateMedia(tokenRepository persist.TokenRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := UpdateMediaInput{}
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		err := UpdateMedia(c, input, tokenRepository, ethClient, ipfsClient, arweaveClient, storageClient)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})

	}
}

// UpdateMedia will find all of the media content for an addresses NFTs and possibly cache it in a storage bucket
func UpdateMedia(c context.Context, input UpdateMediaInput, tokenRepository persist.TokenRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) error {
	if input.TokenID != "" && input.ContractAddress != "" {
		logrus.Infof("updating media for token %s-%s", input.TokenID, input.ContractAddress)
		tokens, err := tokenRepository.GetByTokenIdentifiers(c, input.TokenID, input.ContractAddress, 1, 0)
		if err != nil {
			return err
		}
		token := tokens[0]
		uniqueHandlers := getUniqueMetadataHandlers()
		up, err := getUpdateForToken(c, uniqueHandlers, token.TokenType, token.Chain, token.TokenID, token.ContractAddress, token.TokenMetadata, token.TokenURI, token.Media.MediaType, ethClient, ipfsClient, arweaveClient, storageClient)
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
		tokens, err = tokenRepository.GetByWallet(c, input.OwnerAddress, -1, -1)
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

	if len(tokens) == 0 {
		return nil
	}

	logrus.Infof("Updating %d tokens", len(tokens))

	updates, errChan := updateMediaForTokens(c, tokens, ethClient, ipfsClient, arweaveClient, storageClient)
	for i := 0; i < len(tokens); i++ {
		select {
		case update := <-updates:

			if err := tokenRepository.UpdateByID(c, update.TokenDBID, update.Update); err != nil {
				logrus.WithError(err).Error("failed to update token in database")
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

// updateMediaForTokens will return two channels that will collectively receive the length of the tokens passed in
func updateMediaForTokens(pCtx context.Context, tokens []persist.Token, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) (<-chan tokenUpdateMedia, <-chan error) {
	updateChan := make(chan tokenUpdateMedia)
	errChan := make(chan error)
	wp := workerpool.New(10)

	uniqueHandlers := getUniqueMetadataHandlers()
	for _, t := range tokens {
		token := t
		wp.Submit(func() {

			up, err := getUpdateForToken(pCtx, uniqueHandlers, token.TokenType, token.Chain, token.TokenID, token.ContractAddress, token.TokenMetadata, token.TokenURI, token.Media.MediaType, ethClient, ipfsClient, arweaveClient, storageClient)
			if err != nil {
				errChan <- err
				return
			}

			up.TokenDBID = token.ID

			updateChan <- up

		})
	}
	return updateChan, errChan
}

// TODO Update description, name, etc.
func getUpdateForToken(pCtx context.Context, uniqueHandlers uniqueMetadatas, tokenType persist.TokenType, chain persist.Chain, tokenID persist.TokenID, contractAddress persist.EthereumAddress, metadata persist.TokenMetadata, uri persist.TokenURI, mediaType persist.MediaType, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) (tokenUpdateMedia, error) {
	newMetadata := metadata
	newURI := uri
	if handler, ok := uniqueHandlers[persist.EthereumAddress(contractAddress.String())]; ok {
		logrus.Infof("Using %v metadata handler for %s", handler, contractAddress)
		u, md, err := handler(uri, persist.EthereumAddress(contractAddress.String()), tokenID)
		if err != nil {
			return tokenUpdateMedia{}, fmt.Errorf("failed to get unique metadata for token %s: %s", uri, err)
		}
		newMetadata = md
		newURI = u
	} else {
		if _, ok := newMetadata["error"]; ok || newURI == persist.InvalidTokenURI || mediaType == persist.MediaTypeInvalid {
			logrus.Debugf("skipping token %s-%s", contractAddress, tokenID)
			return tokenUpdateMedia{}, nil
		}

		if newURI.Type() == persist.URITypeNone {
			u, err := rpc.GetTokenURI(pCtx, tokenType, persist.EthereumAddress(contractAddress.String()), tokenID, ethClient)
			if err != nil {
				return tokenUpdateMedia{}, fmt.Errorf("failed to get token URI: %v", err)
			}
			newURI = u
		}

		newURI = newURI.ReplaceID(tokenID)

		if newMetadata == nil || len(newMetadata) == 0 {
			md, err := rpc.GetMetadataFromURI(pCtx, newURI, ipfsClient, arweaveClient)
			if err != nil {
				return tokenUpdateMedia{}, fmt.Errorf("failed to get metadata for token %s: %v", tokenID, err)
			}
			newMetadata = md
		}
	}

	newMedia, err := media.MakePreviewsForMetadata(pCtx, metadata, contractAddress.String(), tokenID, newURI, chain, ipfsClient, arweaveClient, storageClient)
	if err != nil {
		return tokenUpdateMedia{}, fmt.Errorf("failed to make media for token %s-%s: %v", contractAddress, tokenID, err)
	}
	up := tokenUpdateMedia{
		TokenID:         tokenID,
		ContractAddress: persist.EthereumAddress(contractAddress),
		Update: persist.TokenUpdateMediaInput{
			TokenURI: newURI,
			Metadata: newMetadata,
			Media:    newMedia,
		},
	}
	return up, nil
}
