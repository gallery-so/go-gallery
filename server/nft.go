package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

type getNftsByIdInput struct {
	NftId persist.DbId `json:"nft_id" binding:"required"`
}

type getNftsByUserIdInput struct {
	UserId persist.DbId `json:"user_id" binding:"required"`
}

type getNftsOutput struct {
	Nfts []*persist.Nft `json:"nfts"`
}

func getNftsById(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getNftsByIdInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "nft id not found in query values"})
			return
		}

		nfts, err := persist.NeftGetById(input.NftId, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if len(nfts) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("no nfts found with id: %s", input.NftId)})
			return
		}

		if len(nfts) > 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("too many nfts found with id: %s", input.NftId)})
			return
		}
		c.JSON(http.StatusOK, getNftsOutput{Nfts: nfts})
	}
}

// Must specify nft id in json input
func updateNftById(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		nft := &persist.Nft{}
		if err := c.ShouldBindJSON(nft); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		err := persist.NftUpdateById(nft.IDstr, nft, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.Status(http.StatusOK)
	}
}

func getNftsForUser(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getNftsByUserIdInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		nfts, err := persist.NftGetByUserId(input.UserId, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, getNftsOutput{Nfts: nfts})
	}
}

func getUnassignedNftsForUser(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getNftsByUserIdInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		coll, err := persist.CollGetUnassigned(input.UserId, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, getNftsOutput{Nfts: coll.NFTsLst})
	}
}

func getNftsFromOpensea(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		ownerWalletAddr := c.Query("user_id")
		if ownerWalletAddr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "owner wallet address not found in query values"})
			return
		}
		nfts, err := OpenSeaPipelineAssetsForAcc(ownerWalletAddr, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, getNftsOutput{Nfts: nfts})
	}
}
