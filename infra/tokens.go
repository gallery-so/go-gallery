package infra

import (
	"context"
	"math/big"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/mikeydub/go-gallery/util"
)

type getERC721TokensInput struct {
	Address         string `form:"address"`
	ContractAddress string `form:"contract_address"`
	SkipDB          bool   `form:"skip_db"`
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

		tokens := []*persist.Token{}

		if input.Address != "" {
			result, err := getTokensForWallet(c, input.Address, input.SkipDB, pRuntime)
			if err != nil {
				c.JSON(http.StatusInternalServerError, util.ErrorResponse{
					Error: err.Error(),
				})
				return
			}
			tokens = result
		} else if input.ContractAddress != "" {
			result, err := getTokensForContract(c, input.ContractAddress, input.SkipDB, pRuntime)
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

func getTokensForWallet(pCtx context.Context, pWalletAddress string, pSkipDB bool, pRuntime *runtime.Runtime) ([]*persist.Token, error) {
	tokens := []*persist.Token{}
	if !pSkipDB {
		result, err := persist.TokenGetByWallet(pCtx, pWalletAddress, pRuntime)
		if err != nil {
			return nil, err
		}
		if len(result) > 0 {
			go func() {
				sort.Slice(result, func(i, j int) bool {
					b1, ok := new(big.Int).SetString(result[i].LastBlockNum, 16)
					if !ok || !b1.IsUint64() {
						return false
					}
					b2, ok := new(big.Int).SetString(result[j].LastBlockNum, 16)
					if !ok || !b2.IsUint64() {
						return false
					}
					return b1.Uint64() > b2.Uint64()
				})
				GetERC721TokensForWallet(pCtx, pWalletAddress, "0x"+result[0].LastBlockNum, pRuntime)
			}()
		} else {
			result, err = GetERC721TokensForWallet(pCtx, pWalletAddress, "0x0", pRuntime)
			if err != nil {
				return nil, err
			}
		}
		tokens = result
	} else {
		result, err := GetERC721TokensForWallet(pCtx, pWalletAddress, "0x0", pRuntime)
		if err != nil {
			return nil, err
		}
		tokens = result
	}
	return tokens, nil
}
func getTokensForContract(pCtx context.Context, pContractAddress string, pSkipDB bool, pRuntime *runtime.Runtime) ([]*persist.Token, error) {
	tokens := []*persist.Token{}
	if !pSkipDB {
		result, err := persist.TokenGetByContract(pCtx, pContractAddress, pRuntime)
		if err != nil {
			return nil, err
		}

		if len(result) > 0 {
			go func() {
				sort.Slice(result, func(i, j int) bool {
					b1, ok := new(big.Int).SetString(result[i].LastBlockNum, 16)
					if !ok || b1.IsUint64() {
						return false
					}
					b2, ok := new(big.Int).SetString(result[j].LastBlockNum, 16)
					if !ok || !b2.IsUint64() {
						return false
					}
					return b1.Uint64() > b2.Uint64()
				})
				GetERC721TokensForContract(pCtx, pContractAddress, "0x"+result[0].LastBlockNum, pRuntime)
			}()
		} else {
			result, err = GetERC721TokensForContract(pCtx, pContractAddress, "0x0", pRuntime)
			if err != nil {
				return nil, err
			}
		}

		tokens = result
	} else {
		result, err := GetERC721TokensForContract(pCtx, pContractAddress, "0x0", pRuntime)
		if err != nil {
			return nil, err
		}
		tokens = result
	}
	return tokens, nil
}
