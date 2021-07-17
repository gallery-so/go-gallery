package server

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/runtime"
)

func jwtMiddleware(runtime *runtime.Runtime) gin.HandlerFunc {
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
			valid, userAddr, gErr := authJwtVerify(jwt, os.Getenv("JWT_SECRET"), runtime)
			if gErr != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": gErr})
				return
			}

			c.Set("authenticated", valid)
			c.Set("user_addr", userAddr)
		}
		c.Next()
	}
}
