package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

const (
	nftIdQueryNotProvided = "nft id not provided in query values"
)

type getNftsByIdInput struct {
	NftId persist.DbId `json:"id" form:"id" binding:"required"`
}

type getNftsByUserIdInput struct {
	UserId persist.DbId `json:"user_id" form:"user_id" binding:"required"`
}

type getNftsOutput struct {
	Nfts []*persist.Nft `json:"nfts"`
}

func getNftById(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getNftsByIdInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: nftIdQueryNotProvided,
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
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}

		err := persist.NftUpdateById(nft.IDstr, nft, c, pRuntime)
		if err != nil {
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
		ownerWalletAddr := c.Query("addr")
		if ownerWalletAddr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "owner wallet address not found in query values"})
			return
		}
		nfts, err := OpenSeaPipelineAssetsForAcc(ownerWalletAddr, c, pRuntime)
		if len(nfts) == 0 || err != nil {
			nfts = []*persist.Nft{}
		}

		c.JSON(http.StatusOK, getNftsOutput{Nfts: nfts})
	}
}
