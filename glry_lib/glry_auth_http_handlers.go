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
		c.JSON(http.StatusOK, output)
	})

	//-------------------------------------------------------------
	// AUTH_USER_LOGIN
	// UN-AUTHENTICATED

	usersGroup.POST("/login", func(c *gin.Context) {
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
	})

	//-------------------------------------------------------------
	// USER_UPDATE
	// AUTHENTICATED

	usersGroup.POST("/update", func(c *gin.Context) {

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
	})

	//-------------------------------------------------------------
	// USER_GET
	// AUTHENTICATED/UN-AUTHENTICATED

	usersGroup.GET("/get", func(c *gin.Context) {

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

	})

	//-------------------------------------------------------------
	// USER_CREATE
	// UN-AUTHENTICATED

	usersGroup.POST("/create", func(c *gin.Context) {

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

	})

}
