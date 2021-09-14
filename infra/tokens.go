package infra

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/mikeydub/go-gallery/util"
)

type getERC721TokensInput struct {
	Address         string `form:"address"`
	ContractAddress string `form:"contract_address"`
}

type getERC721TokensOutput struct {
	Tokens []*persist.ERC721 `json:"tokens"`
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

		tokens := []*persist.ERC721{}

		if input.Address != "" {
			result, err := persist.ERC721GetByAddress(c, input.Address, pRuntime)
			if len(tokens) == 0 || err != nil {
				tokens = []*persist.ERC721{}
			} else {
				tokens = result
			}
		} else if input.ContractAddress != "" {
			// TODO get for contract
		} else {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: "wallet address or contract address required"})
			return
		}

		c.JSON(http.StatusOK, getERC721TokensOutput{Tokens: tokens})

	}
}
