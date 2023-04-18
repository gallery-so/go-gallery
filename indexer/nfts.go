package indexer

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
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

type getTokenMetadataInput struct {
	TokenID         persist.TokenID         `form:"token_id" binding:"required"`
	ContractAddress persist.EthereumAddress `form:"contract_address" binding:"required"`
	OwnerAddress    persist.EthereumAddress `form:"address"`
}

type GetTokenMetadataOutput struct {
	Metadata persist.TokenMetadata `json:"metadata"`
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
				logger.For(ctx).Errorf("Error getting metadata from URI: %s (%s)", err, newURI)
			} else {
				newMetadata = md
			}
		}

		if newMetadata == nil || len(newMetadata) == 0 {
			util.ErrResponse(c, http.StatusInternalServerError, errNoMetadataFound{Contract: input.ContractAddress, TokenID: input.TokenID})
			return
		}

		if err := nftRepository.UpdateByTokenIdentifiers(ctx, input.TokenID, input.ContractAddress, persist.TokenUpdateMetadataFieldsInput{
			Metadata: newMetadata,
			TokenURI: newURI,
		}); err != nil {
			logger.For(ctx).Errorf("Error updating token metadata: %s (uri: %s)", err, newURI)
		}

		c.JSON(http.StatusOK, GetTokenMetadataOutput{Metadata: newMetadata})
	}
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
