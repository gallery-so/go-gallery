package indexer

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
)

// GetContractOutput is the response for getting a single smart contract
type GetContractOutput struct {
	Contract persist.Contract `json:"contract"`
}

// GetContractInput is the input to the Get Contract endpoint
type GetContractInput struct {
	Address persist.EthereumAddress `form:"address,required"`
}

// UpdateContractMediaInput is used to refresh metadata for a given contract
type UpdateContractMediaInput struct {
	Address persist.EthereumAddress `json:"address,required"`
}

func getContract(contractsRepo persist.ContractRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input GetContractInput
		if err := c.ShouldBindQuery(&input); err != nil {
			err = util.ErrInvalidInput{Reason: fmt.Sprintf("must specify 'address' field: %v", err)}
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		contract, err := contractsRepo.GetByAddress(c, input.Address)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, GetContractOutput{Contract: contract})
	}
}

func updateContractMedia(contractsRepo persist.ContractRepository, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input UpdateContractMediaInput
		if err := c.ShouldBindJSON(&input); err != nil {
			err = util.ErrInvalidInput{Reason: fmt.Sprintf("must specify 'address' field: %v", err)}
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		newMetadata, err := rpc.GetTokenContractMetadata(c, input.Address, ethClient)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		latestBlock, err := ethClient.BlockNumber(c)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		up := persist.ContractUpdateInput{
			Name:        persist.NullString(newMetadata.Name),
			Symbol:      persist.NullString(newMetadata.Symbol),
			LatestBlock: persist.BlockNumber(latestBlock),
		}

		timedContext, cancel := context.WithTimeout(c, time.Second*10)
		defer cancel()

		creator, err := rpc.GetContractCreator(timedContext, input.Address, ethClient)
		if err != nil {
			logger.For(c).Errorf("error finding creator address: %v", err)
		} else {
			up.CreatorAddress = creator
		}

		if err := contractsRepo.UpdateByAddress(c, input.Address, up); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}
