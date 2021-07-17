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
			c.JSON(http.StatusOK, gin.H{"error": "nft id not found in query values"})
			return
		}
		nfts, err := persist.NeftGetById(persist.DbId(nftIDstr), c, pRuntime)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"error": fmt.Sprintf("no nfts found with id: %s", nftIDstr)})
			return
		}

		if len(nfts) == 0 {
			c.JSON(http.StatusOK, gin.H{"error": fmt.Sprintf("no nfts found with id: %s", nftIDstr)})
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
			c.JSON(http.StatusOK, gin.H{"error": err.Error()})
			return
		}

		err := persist.NftUpdateById(nft.IDstr, nft, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusOK, gin.H{"error": "user id not found in query values"})
			return
		}
		nfts, err := persist.NftGetByUserId(persist.DbId(userId), c, pRuntime)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"error": fmt.Sprintf("no nfts found with user_id: %s", userId)})
			return
		}
		if len(nfts) == 0 {
			c.JSON(http.StatusOK, gin.H{"error": fmt.Sprintf("no nfts found with user_id: %s", userId)})
			return
		}

		c.JSON(http.StatusOK, GetNftsForUserResponse{Nfts: nfts})
	}
}

func getUnassignedNftsForUser(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := c.Query("user_id")
		if userId == "" {
			c.JSON(http.StatusOK, gin.H{"error": "user id not found in query values"})
			return
		}
		coll, err := persist.CollGetUnassigned(persist.DbId(userId), c, pRuntime)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"error": fmt.Sprintf("no nfts found with user_id: %s", userId)})
			return
		}
		if len(coll.NFTsLst) == 0 {
			c.JSON(http.StatusOK, gin.H{"error": fmt.Sprintf("no nfts found with user_id: %s", userId)})
			return
		}

		c.JSON(http.StatusOK, GetNftsForUserResponse{Nfts: coll.NFTsLst})
	}
}

func getNftsFromOpensea(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		ownerWalletAddr := c.Query("addr")
		if ownerWalletAddr == "" {
			c.JSON(http.StatusOK, gin.H{"error": "owner wallet address not found in query values"})
			return
		}
		nfts, err := OpenSeaPipelineAssetsForAcc(ownerWalletAddr, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"error": fmt.Sprintf("no nfts found with wallet: %s", ownerWalletAddr)})
			return
		}
		if len(nfts) == 0 {
			c.JSON(http.StatusOK, gin.H{"error": fmt.Sprintf("no nfts found with wallet: %s", ownerWalletAddr)})
			return
		}

		c.JSON(http.StatusOK, gin.H{"nfts": nfts})
	}
}
