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

func getTokenMetadata(ipfsClient *shell.Shell, ethClient *ethclient.Client, arweaveClient *goar.Client) gin.HandlerFunc {
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

		tokenType, err := getTokenType(c, input.TokenID, input.ContractAddress, input.OwnerAddress, ethClient)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		asEthAddress := persist.EthereumAddress(input.ContractAddress.String())
		handler, hasCustomHandler := uniqueMetadataHandlers[asEthAddress]

		newURI, err := rpc.GetTokenURI(ctx, tokenType, input.ContractAddress, input.TokenID, ethClient)
		// It's possible to fetch metadata for some contracts even if URI data is missing.
		if !hasCustomHandler && (err != nil || newURI == "") {
			util.ErrResponse(c, http.StatusInternalServerError, errNoMetadataFound{Contract: input.ContractAddress, TokenID: input.TokenID})
			return
		}

		var newMetadata persist.TokenMetadata
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

		c.JSON(http.StatusOK, GetTokenMetadataOutput{Metadata: newMetadata})
	}
}

func getTokenType(pCtx context.Context, tokenID persist.TokenID, contractAddress, ownerAddress persist.EthereumAddress, ec *ethclient.Client) (t persist.TokenType, err error) {

	if handler, ok := customManualIndex[persist.EthereumAddress(contractAddress.String())]; ok {
		handledToken, err := handler(pCtx, tokenID, ownerAddress, ec)
		if err != nil {
			return t, err
		}
		return handledToken.TokenType, nil
	}

	e721, err := contracts.NewIERC721Caller(contractAddress.Address(), ec)
	if err != nil {
		return "", err
	}
	e1155, err := contracts.NewIERC1155Caller(contractAddress.Address(), ec)
	if err != nil {
		return "", err
	}
	e721Exists, err := e721.SupportsInterface(&bind.CallOpts{Context: pCtx}, [4]byte{0x80, 0x41, 0x67, 0x35})
	if err == nil && e721Exists {
		return persist.TokenTypeERC721, nil
	}

	e1155Exists, err := e1155.SupportsInterface(&bind.CallOpts{Context: pCtx}, [4]byte{0xd9, 0x5a, 0x49, 0x81})
	if err == nil && e1155Exists {
		return persist.TokenTypeERC1155, nil
	}

	return "", fmt.Errorf("failed to get token type for token %s-%s: %s", contractAddress, tokenID, err)

}
