package indexer

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
)

func init() {
	env.RegisterValidation("ALCHEMY_API_URL", "required")
}

// GetContractOutput is the response for getting a single smart contract
type GetContractOutput struct {
	Contract persist.Contract `json:"contract"`
}

// GetContractsOutput is the response for getting multiple smart contracts
type GetContractsOutput struct {
	Contracts []persist.Contract `json:"contracts"`
}

// GetContractInput is the input to the Get Contract endpoint
type GetContractInput struct {
	Address persist.EthereumAddress `form:"address"`
	Owner   persist.EthereumAddress `form:"owner"`
}

// UpdateContractMetadataInput is used to refresh metadata for a given contract
type UpdateContractMetadataInput struct {
	Address persist.EthereumAddress `json:"address,required"`
}

func getContract(contractsRepo persist.ContractRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input GetContractInput
		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		if input.Address != "" {
			contract, err := contractsRepo.GetByAddress(c, input.Address)
			if err != nil {
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}
			c.JSON(http.StatusOK, GetContractOutput{Contract: contract})
			return
		} else if input.Owner != "" {
			contracts, err := contractsRepo.GetContractsOwnedByAddress(c, input.Owner)
			if err != nil {
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}

			c.JSON(http.StatusOK, GetContractsOutput{Contracts: contracts})
			return
		}

		err := util.ErrInvalidInput{Reason: "must specify 'address' or 'owner' field"}
		util.ErrResponse(c, http.StatusBadRequest, err)
	}
}

func updateContractMetadata(contractsRepo persist.ContractRepository, ethClient *ethclient.Client, httpClient *http.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input UpdateContractMetadataInput
		if err := c.ShouldBindJSON(&input); err != nil {
			err = util.ErrInvalidInput{Reason: fmt.Sprintf("must specify 'address' field: %v", err)}
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		err := updateMetadataForContract(c, input, ethClient, httpClient, contractsRepo)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func updateMetadataForContract(c context.Context, input UpdateContractMetadataInput, ethClient *ethclient.Client, httpClient *http.Client, contractsRepo persist.ContractRepository) error {
	newMetadata, err := rpc.GetTokenContractMetadata(c, input.Address, ethClient)
	if err != nil {
		return err
	}

	latestBlock, err := ethClient.BlockNumber(c)
	if err != nil {
		return err
	}

	up := persist.ContractUpdateInput{
		Name:        persist.NullString(newMetadata.Name),
		Symbol:      persist.NullString(newMetadata.Symbol),
		LatestBlock: persist.BlockNumber(latestBlock),
	}

	timedContext, cancel := context.WithTimeout(c, time.Second*10)
	defer cancel()

	owner, _, err := GetContractOwner(timedContext, input.Address, ethClient, httpClient)
	if err != nil {
		logger.For(c).WithError(err).Errorf("error finding creator address")
	} else {
		up.OwnerAddress = owner
	}

	// TODO creator address

	return contractsRepo.UpdateByAddress(c, input.Address, up)
}

type contractOwnerMethod int

const (
	contractOwnerMethodFailed contractOwnerMethod = iota
	contractOwnerMethodOwnable
	contractOwnerMethodAlchemy
	contractOwnerBinarySearch
)

func GetContractOwner(ctx context.Context, address persist.EthereumAddress, ethClient *ethclient.Client, httpClient *http.Client) (persist.EthereumAddress, contractOwnerMethod, error) {

	owner, err := rpc.GetContractOwner(ctx, address, ethClient)
	if err == nil {
		return owner, contractOwnerMethodOwnable, nil
	}
	logger.For(ctx).WithError(err).Error("error finding owner address through ownable interface")

	creator, err := rpc.GetContractCreator(ctx, address, ethClient)
	if err != nil {
		logger.For(ctx).WithError(err).Error("error finding creator address with binary search")
		return "", contractOwnerMethodFailed, err
	}
	return creator, contractOwnerBinarySearch, nil
}
