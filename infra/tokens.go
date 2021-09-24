package infra

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/queue"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/mikeydub/go-gallery/util"
)

const defaultERC721Block = "4BB2B1" // by default start at ERC721 start
const defaultERC1155Block = "57EA7F"

type getERC721TokensInput struct {
	Address         string `form:"address"`
	ContractAddress string `form:"contract_address"`
	SkipDB          bool   `form:"skip_db"`
	PageNumber      int    `form:"page"`
	MaxCount        int    `form:"max"`
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

		if input.MaxCount == 0 {
			input.MaxCount = 50
		}

		tokens := []*persist.Token{}

		if input.Address != "" {
			result, err := GetTokensForWallet(c, input.Address, input.PageNumber, input.MaxCount, input.SkipDB, pRuntime)
			if err != nil {
				c.JSON(http.StatusInternalServerError, util.ErrorResponse{
					Error: err.Error(),
				})
				return
			}
			tokens = result
		} else if input.ContractAddress != "" {
			result, err := GetTokensForContract(c, input.ContractAddress, input.PageNumber, input.MaxCount, input.SkipDB, pRuntime)
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
func GetTokensForWallet(pCtx context.Context, pWalletAddress string, pPageNumber, pMaxCount int, pSkipDB bool, pRuntime *runtime.Runtime) ([]*persist.Token, error) {
	accounts, _ := persist.AccountGetByAddress(pCtx, pWalletAddress, pRuntime)
	lastSyncedBlock := defaultERC721Block
	if len(accounts) > 0 {
		account := accounts[0]
		lastSyncedBlock = account.LastSyncedBlock

		if time.Since(account.LastUpdated.Time()) > persist.TTB {
			pSkipDB = true
		}
	}
	if !pSkipDB {
		result, err := persist.TokenGetByWallet(pCtx, pWalletAddress, pPageNumber, pMaxCount, pRuntime)
		if err != nil {
			return nil, err
		}

		if len(result) < pMaxCount {
			next, err := getTokensFromBCForWallet(pCtx, pWalletAddress, pPageNumber, pMaxCount, lastSyncedBlock, true, pRuntime)
			if err != nil {
				return nil, err
			}
			if len(next) > pMaxCount-len(result) {
				next = next[:pMaxCount-len(result)]
			}
			result = append(result, next...)

			return result, nil
		}
		pRuntime.BlockchainUpdateQueue.AddJob(queue.Job{
			Name: "update wallet tokens",
			Action: func() error {
				_, err := getTokensFromBCForWallet(pCtx, pWalletAddress, 0, 0, lastSyncedBlock, false, pRuntime)
				if err != nil {
					return err
				}
				return nil
			},
		})
		return result[:pMaxCount], nil
	}
	result, err := getTokensFromBCForWallet(pCtx, pWalletAddress, pPageNumber, pMaxCount, lastSyncedBlock, true, pRuntime)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetTokensForContract returns the tokens for a contract from the DB if possible while also queuing an
// update of DB from the block chain, or goes straight to the block chain if the DB returns no results
func GetTokensForContract(pCtx context.Context, pContractAddress string, pPageNumber int, pMaxCount int, pSkipDB bool, pRuntime *runtime.Runtime) ([]*persist.Token, error) {
	accounts, _ := persist.ContractGetByAddress(pCtx, pContractAddress, pRuntime)
	lastSyncedBlock := defaultERC721Block
	if len(accounts) > 0 {
		account := accounts[0]
		lastSyncedBlock = account.LastSyncedBlock

		if time.Since(account.LastUpdated.Time()) > persist.TTB {
			pSkipDB = true
		}
	}
	if !pSkipDB {
		result, err := persist.TokenGetByContract(pCtx, pContractAddress, pPageNumber, pMaxCount, pRuntime)
		if err != nil {
			return nil, err
		}

		if len(result) < pMaxCount {
			next, err := getTokensFromBCForContract(pCtx, pContractAddress, pPageNumber, pMaxCount, lastSyncedBlock, true, pRuntime)
			if err != nil {
				return nil, err
			}
			if len(next) > pMaxCount-len(result) {
				next = next[:pMaxCount-len(result)]
			}
			result = append(result, next...)

			return result, nil
		}
		pRuntime.BlockchainUpdateQueue.AddJob(queue.Job{
			Name: "update wallet tokens",
			Action: func() error {
				_, err := getTokensFromBCForContract(pCtx, pContractAddress, 0, 0, lastSyncedBlock, false, pRuntime)
				if err != nil {
					return err
				}
				return nil
			},
		})
		return result[:pMaxCount], nil
	}
	result, err := getTokensFromBCForContract(pCtx, pContractAddress, pPageNumber, pMaxCount, lastSyncedBlock, true, pRuntime)
	if err != nil {
		return nil, err
	}
	return result, nil
}
