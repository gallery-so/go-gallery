package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/copy"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

type getNftsByIdInput struct {
	NftId persist.DbId `json:"id" form:"id" binding:"required"`
}

type getNftsByUserIdInput struct {
	UserId persist.DbId `json:"user_id" form:"user_id" binding:"required"`
}

type getOpenseaNftsInput struct {
	WalletAddress string `json:"addr" form:"addr" binding:"required"`
}

type getNftsOutput struct {
	Nfts []*persist.Nft `json:"nfts"`
}

func getNftById(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getNftsByIdInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: copy.NftIdQueryNotProvided,
			})
			return
		}

		nfts, err := persist.NftGetById(input.NftId, c, pRuntime)
		if len(nfts) == 0 || err != nil {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: fmt.Sprintf("no nfts found with id: %s", input.NftId),
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
func updateNftById(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		nft := &persist.Nft{}
		if err := c.ShouldBindJSON(nft); err != nil {
			// TODO: think about how to log errors
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		userId, ok := getUserIdFromCtx(c)
		if !ok {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "user id not found in context"})
			return
		}

		err := persist.NftUpdateById(nft.IDstr, userId, nft, c, pRuntime)
		if err != nil {
			if err.Error() == copy.CouldNotFindDocument {
				c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}

		c.Status(http.StatusOK)
	}
}

func getNftsForUser(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getNftsByUserIdInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		nfts, err := persist.NftGetByUserId(input.UserId, c, pRuntime)
		if len(nfts) == 0 || err != nil {
			nfts = []*persist.Nft{}
		}

		c.JSON(http.StatusOK, getNftsOutput{Nfts: nfts})
	}
}

func getUnassignedNftsForUser(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getNftsByUserIdInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		coll, err := persist.CollGetUnassigned(input.UserId, c, pRuntime)
		if coll == nil || err != nil {
			coll = &persist.Collection{NftsLst: []*persist.Nft{}}
		}

		c.JSON(http.StatusOK, getNftsOutput{Nfts: coll.NftsLst})
	}
}

func getNftsFromOpensea(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getOpenseaNftsInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		nfts, err := OpenSeaPipelineAssetsForAcc(input.WalletAddress, c, pRuntime)
		if len(nfts) == 0 || err != nil {
			nfts = []*persist.Nft{}
		}

		c.JSON(http.StatusOK, getNftsOutput{Nfts: nfts})
	}
}
