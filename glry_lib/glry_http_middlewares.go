package glry_lib

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/glry_core"
)

type authContextValue struct {
	AuthenticatedBool bool
	UserAddressStr    string
}

func getAuthFromCtx(c *gin.Context) (authContextValue, bool) {
	auth, ok := c.Get("auth")
	if !ok {
		return authContextValue{}, false
	}
	if authStruct, ok := auth.(authContextValue); ok {
		return authStruct, true
	} else {
		return authContextValue{}, false
	}

}

func jwtMiddleware(runtime *glry_core.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeaders := strings.Split(c.GetHeader("Authorization"), " ")
		if len(authHeaders) > 0 && len(authHeaders) < 2 {
			if authHeaders[0] != "Bearer" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid authorization header format"})
				return
			}
			// get string after "Bearer"
			jwt := authHeaders[1]
			// use an env variable as jwt secret as upposed to using a stateful secret stored in
			// database that is unique to every user and session
			valid, userAddr, gErr := AuthJWTverify(jwt, os.Getenv("JWT_SECRET"), runtime)
			if gErr != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": gErr})
				return
			}

			c.Set("auth", authContextValue{
				AuthenticatedBool: valid,
				UserAddressStr:    userAddr,
			})
		}
		c.Next()
	}
}
