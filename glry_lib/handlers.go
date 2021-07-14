package glry_lib

import (
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
		output, err := CollGetPipeline(input, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		output, err := CollCreatePipeline(input, input.OwnerUserIdStr, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err})
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

		output, err := CollDeletePipeline(input, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		nfts, err := glry_db.NFTgetByID(nftIDstr, c, pRuntime)
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
func updateNftById(pRuntime *glry_core.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		nft := &glry_db.GLRYnft{}
		if err := c.ShouldBindJSON(nft); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		err := glry_db.NFTupdateById(string(nft.IDstr), nft, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		nfts, err := glry_db.NFTgetByUserID(userId, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	Env     string `json:"env"`
}

func healthcheck(pRuntime *glry_core.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, HealthcheckResponse{
			Message: "gallery operational",
			Env:     pRuntime.Config.EnvStr,
		})
	}
}

func getAuthPreflight(pRuntime *glry_core.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		userAddrStr := c.Query("addr")
		input := &GLRYauthUserGetPreflightInput{
			AddressStr: glry_db.GLRYuserAddress(userAddrStr),
		}
		// GET_PUBLIC_INFO
		output, gErr := AuthUserGetPreflightPipeline(input, c, pRuntime)
		if gErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gErr})
			return
		}

		//------------------
		// OUTPUT
		c.JSON(http.StatusOK, output)
	}
}

func login(pRuntime *glry_core.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &GLRYauthUserLoginInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		//------------------

		// USER_LOGIN__PIPELINE
		output, gErr := AuthUserLoginAndMemorizeAttemptPipeline(input,
			c.Request,
			c,
			pRuntime)
		if gErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gErr})
			return
		}

		/*
			// ADD!! - going forward we should follow this approach, after v1
			// SET_JWT_COOKIE
			expirationTime := time.Now().Add(time.Duration(pRuntime.Config.JWTtokenTTLsecInt/60) * time.Minute)
			http.SetCookie(pResp, &http.Cookie{
				Name:    "glry_token",
				Value:   userJWTtokenStr,
				Expires: expirationTime,
			})*/

		//------------------
		// OUTPUT
		c.JSON(http.StatusOK, output)
	}
}

func updateUserAuth(pRuntime *glry_core.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		if auth := c.GetBool("authenticated"); !auth {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization required"})
			return
		}

		up := &GLRYauthUserUpdateInput{}

		if err := c.ShouldBindJSON(up); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		//------------------
		// UPDATE
		gErr := AuthUserUpdatePipeline(up, c, pRuntime)
		if gErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gErr})
			return
		}
		//------------------
		// OUTPUT
		c.Status(http.StatusOK)
	}
}

func getUserAuth(pRuntime *glry_core.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		auth := c.GetBool("authenticated")

		userAddrStr := c.Query("addr")
		input := &GLRYauthUserGetInput{
			AddressStr: glry_db.GLRYuserAddress(userAddrStr),
		}

		output, gErr := AuthUserGetPipeline(input,
			auth,
			c, pRuntime)
		if gErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gErr})
			return
		}

		//------------------
		// OUTPUT

		c.JSON(http.StatusOK, output)

	}
}

func createUser(pRuntime *glry_core.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &GLRYauthUserCreateInput{}

		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		//------------------
		// USER_CREATE
		output, gErr := AuthUserCreatePipeline(input, c, pRuntime)
		if gErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gErr})
			return
		}

		//------------------
		// OUTPUT

		c.JSON(http.StatusOK, output)

	}
}
