package glry_lib

import (
	"fmt"

	// "time"
	"context"
	"net/http"

	// log "github.com/sirupsen/logrus"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	"github.com/mikeydub/go-gallery/glry_extern_services"
)

//-------------------------------------------------------------
func HandlersInit(pRuntime *glry_core.Runtime) {

	apiGroupV1 := pRuntime.Router.Group("/glry/v1")

	// AUTH_HANDLERS
	AuthHandlersInit(pRuntime, apiGroupV1)

	//-------------------------------------------------------------
	// COLLECTION
	//-------------------------------------------------------------
	// COLLECTION_GET

	apiGroupV1.GET("/collections/get", func(c *gin.Context) {
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

		//------------------
		// OUTPUT

		c.JSON(http.StatusOK, gin.H{"colls": output.CollsOutputsLst})
	})

	//-------------------------------------------------------------
	// COLLECTION_CREATE

	apiGroupV1.POST("/collections/create", func(c *gin.Context) {
		input := &GLRYcollCreateInput{}
		if err := c.BindJSON(input); err != nil {
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

		//------------------
		// OUTPUT
		c.JSON(http.StatusOK, output)
	})

	//-------------------------------------------------------------
	// COLLECTION_DELETE

	apiGroupV1.POST("/collections/delete", func(c *gin.Context) {
		input := &GLRYcollDeleteInput{}
		if err := c.BindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		output, gErr := CollDeletePipeline(input, c, pRuntime)
		if gErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gErr})
			return
		}

		//------------------
		// OUTPUT
		c.JSON(http.StatusOK, output)
	})

	//-------------------------------------------------------------
	// NFTS
	//-------------------------------------------------------------
	// SINGLE GET

	apiGroupV1.GET("/nfts/get", func(c *gin.Context) {
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

		//------------------
		// OUTPUT
		c.JSON(http.StatusOK, gin.H{
			"nfts": nfts,
		})

	})

	// SINGLE UPDATE
	// must specify nft id in json input
	apiGroupV1.POST("/nfts/update", func(c *gin.Context) {
		nft := &glry_db.GLRYnft{}
		if err := c.BindJSON(nft); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		gErr := glry_db.NFTupdateById(string(nft.IDstr), nft, c, pRuntime)
		if gErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gErr})
			return
		}

		c.Status(http.StatusOK)
	})

	// USER_GET
	apiGroupV1.GET("/nfts/user_get", func(c *gin.Context) {
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

		//------------------
		// OUTPUT
		c.JSON(http.StatusOK, gin.H{"nfts": nfts})

	})

	//-------------------------------------------------------------
	// OPENSEA_GET

	// USER_GET
	apiGroupV1.GET("/nfts/opensea_get", func(c *gin.Context) {
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

		//------------------
		// OUTPUT
		c.Status(http.StatusOK)

	})

	//-------------------------------------------------------------
	// VAR
	//-------------------------------------------------------------
	// HEALTH

	apiGroupV1.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"msg": "gallery operational",
			"env": pRuntime.Config.EnvStr,
		})
	})

	//-------------------------------------------------------------
}
