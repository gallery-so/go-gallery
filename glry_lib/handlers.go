package glry_lib

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	"github.com/mikeydub/go-gallery/glry_extern_services"
)

func getAllCollectionsForUser(pRuntime *glry_core.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		userIDstr := c.Query("userid")

		input := &GLRYcollGetInput{
			UserIDstr: glry_db.GLRYuserID(userIDstr),
		}

		//------------------
		// CREATE
		output, gErr := CollGetPipeline(input, context.TODO(), pRuntime)
		if gErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gErr})
			return
		}

		c.JSON(http.StatusOK, gin.H{"colls": output.CollsOutputsLst})
	}
}

func createCollection(pRuntime *glry_core.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &GLRYcollCreateInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		//------------------
		// CREATE
		output, gErr := CollCreatePipeline(input, input.OwnerUserIdStr, c, pRuntime)
		if gErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gErr})
			return
		}

		c.JSON(http.StatusOK, output)
	}
}

func deleteCollection(pRuntime *glry_core.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &GLRYcollDeleteInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		output, gErr := CollDeletePipeline(input, c, pRuntime)
		if gErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gErr})
			return
		}

		c.JSON(http.StatusOK, output)
	}
}

func getNftById(pRuntime *glry_core.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		nftIDstr := c.Query("id")
		if nftIDstr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "nft id not found in query values"})
			return
		}
		nfts, gErr := glry_db.NFTgetByID(nftIDstr, c, pRuntime)
		if gErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gErr})
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
func updateNftById(pRuntime *glry_core.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		nft := &glry_db.GLRYnft{}
		if err := c.ShouldBindJSON(nft); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		gErr := glry_db.NFTupdateById(string(nft.IDstr), nft, c, pRuntime)
		if gErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gErr})
			return
		}

		c.Status(http.StatusOK)
	}
}

type GetNftsForUserResponse struct {
	Nfts []*glry_db.GLRYnft `json:"nfts"`
}

func getNftsForUser(pRuntime *glry_core.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := c.Query("user_id")
		if userId == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user id not found in query values"})
			return
		}
		nfts, gErr := glry_db.NFTgetByUserID(userId, c, pRuntime)
		if gErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gErr})
			return
		}

		c.JSON(http.StatusOK, GetNftsForUserResponse{Nfts: nfts})
	}
}

func getNftsFromOpensea(pRuntime *glry_core.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		ownerWalletAddr := c.Query("user_id")
		if ownerWalletAddr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "owner wallet address not found in query values"})
			return
		}
		_, gErr := glry_extern_services.OpenSeaPipelineAssetsForAcc(ownerWalletAddr, c, pRuntime)
		if gErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gErr})
			return
		}

		c.Status(http.StatusOK)
	}
}

type HealthcheckResponse struct {
	Message string `json:"msg"`
	Env 	string `json:"env"`
}

func healthcheck(pRuntime *glry_core.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, HealthcheckResponse{
			Message: "gallery operational",
			Env: pRuntime.Config.EnvStr,
		})
	}
}