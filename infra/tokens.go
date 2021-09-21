package infra

import (
	"context"
	"fmt"
	"net/http"
	"time"

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
		if input.PageNumber == 0 {
			input.PageNumber = 1
		}
		if input.PageNumber < 0 {
			input.PageNumber = 0
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
	accounts, _ := persist.AccountGetByAddress(pCtx, pWalletAddress, pRuntime)
	lastSyncedBlock := "0"
	if len(accounts) > 0 {
		account := accounts[0]
		lastSyncedBlock = account.LastSyncedBlock

		if time.Since(account.LastUpdated.Time()) > persist.TTB {
			pSkipDB = true
		}
	}
	if !pSkipDB {
		result, err := persist.TokenGetByWallet(pCtx, pWalletAddress, pPageNumber, pRuntime)
		if err != nil {
			return nil, err
		}
		if len(result) > 0 {
			pRuntime.BlockchainUpdateQueue.AddJob(queue.Job{
				Action: func() error {
					_, err := getTokensFromBCForWallet(pCtx, pWalletAddress, pPageNumber, lastSyncedBlock, false, pRuntime)
					return err
				},
				Name: fmt.Sprintf("GetTokensForWallet: %s", pWalletAddress),
			})
			return result, nil
		}
	}
	result, err := getTokensFromBCForWallet(pCtx, pWalletAddress, pPageNumber, lastSyncedBlock, true, pRuntime)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetTokensForContract returns the tokens for a contract from the DB if possible while also queuing an
// update of DB from the block chain, or goes straight to the block chain if the DB returns no results
func GetTokensForContract(pCtx context.Context, pContractAddress string, pPageNumber int, pSkipDB bool, pRuntime *runtime.Runtime) ([]*persist.Token, error) {
	accounts, _ := persist.ContractGetByAddress(pCtx, pContractAddress, pRuntime)
	lastSyncedBlock := "0"
	if len(accounts) > 0 {
		account := accounts[0]
		lastSyncedBlock = account.LastSyncedBlock

		if time.Since(account.LastUpdated.Time()) > persist.TTB {
			pSkipDB = true
		}
	}
	if !pSkipDB {
		result, err := persist.TokenGetByContract(pCtx, pContractAddress, pPageNumber, pRuntime)
		if err != nil {
			return nil, err
		}

		if len(result) > 0 {
			pRuntime.BlockchainUpdateQueue.AddJob(queue.Job{
				Action: func() error {
					_, err := getTokensFromBCForContract(pCtx, pContractAddress, pPageNumber, lastSyncedBlock, false, pRuntime)
					return err
				},
				Name: fmt.Sprintf("GetTokensForContract: %s", pContractAddress),
			})
			return result, nil
		}
	}
	result, err := getTokensFromBCForContract(pCtx, pContractAddress, pPageNumber, lastSyncedBlock, true, pRuntime)
	if err != nil {
		return nil, err
	}
	return result, nil
}
