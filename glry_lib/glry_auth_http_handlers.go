package glry_lib

import (
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/glry_core"
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
	usersGroup.GET("/auth/get_preflight", getAuthPreflight(pRuntime))

	//-------------------------------------------------------------
	// AUTH_USER_LOGIN
	// UN-AUTHENTICATED

	usersGroup.POST("/login", login(pRuntime))

	//-------------------------------------------------------------
	// USER_UPDATE
	// AUTHENTICATED

	usersGroup.POST("/update", updateUserAuth(pRuntime))

	//-------------------------------------------------------------
	// USER_GET
	// AUTHENTICATED/UN-AUTHENTICATED

	usersGroup.GET("/get", getUserAuth(pRuntime))

	//-------------------------------------------------------------
	// USER_CREATE
	// UN-AUTHENTICATED

	usersGroup.POST("/create", createUser(pRuntime))

}
