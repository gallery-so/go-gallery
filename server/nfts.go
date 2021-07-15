package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

func getNftById(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		nftIDstr := c.Query("id")
		if nftIDstr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "nft id not found in query values"})
			return
		}
		nfts, err := persist.NeftGetById(persist.DbId(nftIDstr), c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if len(nfts) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("no nfts found with id: %s", nftIDstr)})
			return
		}

		// TODO: this should just return a single NFT
		c.JSON(http.StatusOK, gin.H{
			"nfts": nfts,
		})
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

type GetNftsForUserResponse struct {
	Nfts []*persist.Nft `json:"nfts"`
}

func getNftsForUser(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := c.Query("user_id")
		if userId == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user id not found in query values"})
			return
		}
		nfts, err := persist.NftGetByUserId(persist.DbId(userId), c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, GetNftsForUserResponse{Nfts: nfts})
	}
}

func getNftsFromOpensea(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		ownerWalletAddr := c.Query("user_id")
		if ownerWalletAddr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "owner wallet address not found in query values"})
			return
		}
		_, gErr := OpenSeaPipelineAssetsForAcc(ownerWalletAddr, c, pRuntime)
		if gErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gErr})
			return
		}

		c.Status(http.StatusOK)
	}
}
