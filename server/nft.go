package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/copy"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

type getNftsByIDInput struct {
	NftID persist.DbID `json:"id" form:"id" binding:"required"`
}

type getNftsByUserIDInput struct {
	UserID persist.DbID `json:"user_id" form:"user_id" binding:"required"`
}

type getOpenseaNftsInput struct {
	WalletAddress string `json:"address" form:"address" binding:"required"`
}

type getNftsOutput struct {
	Nfts []*persist.Nft `json:"nfts"`
}

func getNftByID(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getNftsByIDInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{
				Error: copy.NftIDQueryNotProvided,
			})
			return
		}

		nfts, err := persist.NftGetByID(c, input.NftID, pRuntime)
		if len(nfts) == 0 || err != nil {
			c.JSON(http.StatusNotFound, errorResponse{
				Error: fmt.Sprintf("no nfts found with id: %s", input.NftID),
			})
			return
		}

		if len(nfts) > 1 {
			nfts = nfts[:1]
			// TODO log that this should not be happening
		}
		c.JSON(http.StatusOK, nfts[0])
	}
}

// Must specify nft id in json input
func updateNftByID(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		nft := &persist.Nft{}
		if err := c.ShouldBindJSON(nft); err != nil {
			// TODO: think about how to log errors
			c.JSON(http.StatusBadRequest, errorResponse{
				Error: err.Error(),
			})
			return
		}

		userID, ok := getUserIDfromCtx(c)
		if !ok {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "user id not found in context"})
			return
		}

		err := persist.NftUpdateByID(c, nft.ID, userID, nft, pRuntime)
		if err != nil {
			if err.Error() == copy.CouldNotFindDocument {
				c.JSON(http.StatusNotFound, errorResponse{Error: err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, successOutput{Success: true})
	}
}

func getNftsForUser(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getNftsByUserIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		nfts, err := persist.NftGetByUserID(c, input.UserID, pRuntime)
		if len(nfts) == 0 || err != nil {
			nfts = []*persist.Nft{}
		}

		c.JSON(http.StatusOK, getNftsOutput{Nfts: nfts})
	}
}

func getUnassignedNftsForUser(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getNftsByUserIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		coll, err := persist.CollGetUnassigned(c, input.UserID, pRuntime)
		if coll == nil || err != nil {
			coll = &persist.Collection{Nfts: []*persist.Nft{}}
		}

		c.JSON(http.StatusOK, getNftsOutput{Nfts: coll.Nfts})
	}
}

func getNftsFromOpensea(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getOpenseaNftsInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		nfts, err := openSeaPipelineAssetsForAcc(c, input.WalletAddress, pRuntime)
		if len(nfts) == 0 || err != nil {
			nfts = []*persist.Nft{}
		}

		c.JSON(http.StatusOK, getNftsOutput{Nfts: nfts})
	}
}
