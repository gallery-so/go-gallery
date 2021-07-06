package glry_lib

import (
	// "fmt"

	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	// "github.com/davecgh/go-spew/spew"
)

//-------------------------------------------------------------
func AuthHandlersInit(pRuntime *glry_core.Runtime, parent *gin.RouterGroup) {

	usersGroup := parent.Group("/users")
	usersGroup.Use(jwtMiddleware(pRuntime))

	//-------------------------------------------------------------
	// AUTH_GET_PREFLIGHT
	// UN-AUTHENTICATED

	// called before login/sugnup calls, mostly to get nonce and also discover if user exists.

	// [GET] /glry/v1/auth/get_preflight?addr=:walletAddress
	usersGroup.GET("/auth/get_preflight", func(c *gin.Context) {
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
		c.JSON(http.StatusOK, gin.H{
			"nonce":       output.NonceStr,
			"user_exists": output.UserExistsBool,
		})
	})

	//-------------------------------------------------------------
	// AUTH_USER_LOGIN
	// UN-AUTHENTICATED

	usersGroup.POST("/login", func(c *gin.Context) {
		input := &GLRYauthUserLoginInput{}
		if err := c.BindJSON(input); err != nil {
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

		// FAILED - INVALID_SIGNATURE
		if !output.SignatureValidBool {
			c.JSON(http.StatusBadRequest, gin.H{
				"sig_valid": false,
			})
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
		c.JSON(http.StatusOK, gin.H{
			"jwt_token": output.JWTtokenStr,
			"user_id":   output.UserIDstr,
		})
	})

	//-------------------------------------------------------------
	// USER_UPDATE
	// AUTHENTICATED

	usersGroup.POST("/update", func(c *gin.Context) {

		if auth, ok := getAuthFromCtx(c); !ok || !auth.AuthenticatedBool {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization required"})
			return
		}

		up := &GLRYauthUserUpdateInput{}

		if err := c.BindJSON(up); err != nil {
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
	})

	//-------------------------------------------------------------
	// USER_GET
	// AUTHENTICATED/UN-AUTHENTICATED

	usersGroup.GET("/get", func(c *gin.Context) {

		auth, _ := getAuthFromCtx(c)

		userAddrStr := c.Query("addr")
		input := &GLRYauthUserGetInput{
			AddressStr: glry_db.GLRYuserAddress(userAddrStr),
		}

		output, gErr := AuthUserGetPipeline(input,
			auth.AuthenticatedBool,
			c, pRuntime)
		if gErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gErr})
			return
		}

		//------------------
		// OUTPUT

		c.JSON(http.StatusOK, output)

	})

	//-------------------------------------------------------------
	// USER_CREATE
	// UN-AUTHENTICATED

	usersGroup.POST("/create", func(c *gin.Context) {

		input := &GLRYauthUserCreateInput{}

		if err := c.BindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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

		c.JSON(http.StatusOK, gin.H{
			"sig_valid": output.SignatureValidBool,
			"jwt_token": output.JWTtokenStr,
			"user_id":   output.UserIDstr,
		})

	})

}
