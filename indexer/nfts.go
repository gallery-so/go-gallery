package indexer

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

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

		ctx, cancel := context.WithTimeout(ctx, time.Minute*2)
		defer cancel()

		asEthAddress := persist.EthereumAddress(input.ContractAddress.String())
		handler, hasCustomHandler := uniqueMetadataHandlers[asEthAddress]

		newURI, err := rpc.RetryGetTokenURI(ctx, "", input.ContractAddress, input.TokenID, ethClient)
		// It's possible to fetch metadata for some contracts even if URI data is missing.
		if !hasCustomHandler && (err != nil || newURI == "") {
			var status int
			if err != nil {
				switch err.(type) {
				case rpc.ErrTokenURINotFound:
					status = http.StatusNotFound
				default:
					status = http.StatusInternalServerError
				}
			}
			util.ErrResponse(c, status, errNoMetadataFound{Contract: input.ContractAddress, TokenID: input.TokenID})
			return
		}

		var newMetadata persist.TokenMetadata
		if hasCustomHandler {
			logger.For(ctx).Infof("Using %v metadata handler for %s", handler, input.ContractAddress)
			newURI, newMetadata, err = handler(ctx, newURI, asEthAddress, input.TokenID, ethClient, ipfsClient, arweaveClient)
		} else if newURI != "" {
			newMetadata, err = rpc.GetMetadataFromURI(ctx, newURI, ipfsClient, arweaveClient)
		}

		if err != nil || len(newMetadata) == 0 {
			logger.For(ctx).Errorf("Error getting metadata from URI: %s (%s)", err, util.TruncateWithEllipsis(newURI.String(), 50))
			status := http.StatusNotFound
			if err != nil {
				switch caught := err.(type) {
				case util.ErrHTTP:
					if caught.Status != http.StatusNotFound {
						status = http.StatusInternalServerError
					}
				case *url.Error, *net.DNSError, *shell.Error:
					// do nothing
				default:
					status = http.StatusInternalServerError
				}
			}
			util.ErrResponse(c, status, errNoMetadataFound{Contract: input.ContractAddress, TokenID: input.TokenID})
			return
		}

		c.JSON(http.StatusOK, GetTokenMetadataOutput{Metadata: newMetadata})
	}
}
