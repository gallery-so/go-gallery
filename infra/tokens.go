package infra

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/queue"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/mikeydub/go-gallery/util"
)

type getERC721TokensInput struct {
	Address         string `form:"address"`
	ContractAddress string `form:"contract_address"`
	SkipDB          bool   `form:"skip_db"`
	PageNumber      int    `form:"page"`
}

type getERC721TokensOutput struct {
	Tokens []*persist.Token `json:"tokens"`
}

func getERC721Tokens(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &getERC721TokensInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{
				Error: err.Error(),
			})
			return
		}
		if input.PageNumber < 1 {
			input.PageNumber = 1
		}

		tokens := []*persist.Token{}

		if input.Address != "" {
			result, err := GetTokensForWallet(c, input.Address, input.PageNumber, input.SkipDB, pRuntime)
			if err != nil {
				c.JSON(http.StatusInternalServerError, util.ErrorResponse{
					Error: err.Error(),
				})
				return
			}
			tokens = result
		} else if input.ContractAddress != "" {
			result, err := GetTokensForContract(c, input.ContractAddress, input.PageNumber, input.SkipDB, pRuntime)
			if err != nil {
				c.JSON(http.StatusInternalServerError, util.ErrorResponse{
					Error: err.Error(),
				})
				return
			}
			tokens = result
		} else {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: "wallet address or contract address required"})
			return
		}

		c.JSON(http.StatusOK, getERC721TokensOutput{Tokens: tokens})

	}
}

// GetTokensForWallet returns the tokens for a wallet from the DB if possible while also queuing an
// update of DB from the block chain, or goes straight to the block chain if the DB returns no results
func GetTokensForWallet(pCtx context.Context, pWalletAddress string, pPageNumber int, pSkipDB bool, pRuntime *runtime.Runtime) ([]*persist.Token, error) {
	if !pSkipDB {
		result, err := persist.TokenGetByWallet(pCtx, pWalletAddress, pPageNumber, pRuntime)
		if err != nil {
			return nil, err
		}
		if len(result) > 0 {
			pRuntime.BlockchainUpdateQueue.AddJob(queue.Job{
				Action: func() error {
					accounts, _ := persist.AccountGetByAddress(pCtx, pWalletAddress, pRuntime)
					if len(accounts) == 0 {
						_, err := GetTokensFromBCForWallet(pCtx, pWalletAddress, pPageNumber, "0", false, pRuntime)
						return err
					}
					_, err := GetTokensFromBCForWallet(pCtx, pWalletAddress, pPageNumber, accounts[0].LastSyncedBlock, false, pRuntime)
					return err

				},
				Name: fmt.Sprintf("GetTokensForWallet: %s", pWalletAddress),
			})
			return result, nil
		}
	}
	result, err := GetTokensFromBCForWallet(pCtx, pWalletAddress, pPageNumber, "0", true, pRuntime)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetTokensForContract returns the tokens for a contract from the DB if possible while also queuing an
// update of DB from the block chain, or goes straight to the block chain if the DB returns no results
func GetTokensForContract(pCtx context.Context, pContractAddress string, pPageNumber int, pSkipDB bool, pRuntime *runtime.Runtime) ([]*persist.Token, error) {

	if !pSkipDB {
		result, err := persist.TokenGetByContract(pCtx, pContractAddress, pPageNumber, pRuntime)
		if err != nil {
			return nil, err
		}

		if len(result) > 0 {
			pRuntime.BlockchainUpdateQueue.AddJob(queue.Job{
				Action: func() error {
					accounts, _ := persist.AccountGetByAddress(pCtx, pContractAddress, pRuntime)
					if len(accounts) == 0 {
						_, err := GetTokensFromBCForWallet(pCtx, pContractAddress, pPageNumber, "0", true, pRuntime)
						return err
					}
					_, err := GetTokensFromBCForWallet(pCtx, pContractAddress, pPageNumber, accounts[0].LastSyncedBlock, false, pRuntime)
					return err
				},
				Name: fmt.Sprintf("GetTokensForContract: %s", pContractAddress),
			})
			return result, nil
		}
	}
	result, err := GetTokensFromBCForContract(pCtx, pContractAddress, pPageNumber, "0", true, pRuntime)
	if err != nil {
		return nil, err
	}
	return result, nil
}
