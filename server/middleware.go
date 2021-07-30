package server

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/copy"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

const (
	userIdContextKey = "user_id"
	authContextKey   = "authenticated"
)

func jwtRequired(runtime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{Error: copy.InvalidAuthHeader})
			return
		}
		authHeaders := strings.Split(header, " ")
		if len(authHeaders) == 2 {
			if authHeaders[0] != "Bearer" {
				c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{Error: copy.InvalidAuthHeader})
				return
			}
			// get string after "Bearer"
			jwt := authHeaders[1]
			// use an env variable as jwt secret as upposed to using a stateful secret stored in
			// database that is unique to every user and session
			valid, userId, err := authJwtParse(jwt, os.Getenv("JWT_SECRET"), runtime)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: err.Error()})
				return
			}

			if !valid {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid jwt"})
				return
			}

			c.Set(userIdContextKey, userId)
		} else {
			c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{Error: copy.InvalidAuthHeader})
			return
		}
		c.Next()
	}
}

func jwtOptional(runtime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header != "" {
			authHeaders := strings.Split(c.GetHeader("Authorization"), " ")
			if len(authHeaders) == 2 {
				// get string after "Bearer"
				jwt := authHeaders[1]
				valid, userId, _ := authJwtParse(jwt, os.Getenv("JWT_SECRET"), runtime)
				c.Set(authContextKey, valid)
				c.Set(userIdContextKey, userId)
			}
		}
		c.Next()
	}
}

func handleCORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO make origin url env specific
		c.Writer.Header().Set("Access-Control-Allow-Origin", "https://gallery-dev.vercel.app")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func getUserIdFromCtx(c *gin.Context) (persist.DbId, bool) {
	val, ok := c.Get(userIdContextKey)
	if !ok {
		return "", false
	}
	userId, ok := val.(persist.DbId)
	if !ok {
		return "", false
	}
	return userId, true
}
