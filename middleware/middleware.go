package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/eth"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	// UserIDContextKey is the key for retrieving the user id from the context
	UserIDContextKey = "user_id"
	// AuthContextKey is the key for retrieving the auth status from the context
	AuthContextKey = "authenticated"
)

var rateLimiter = newIPRateLimiter(1, 5)

// ErrInvalidJWT is returned when the JWT is invalid
var ErrInvalidJWT = errors.New("invalid JWT")

// ErrRateLimited is returned when the request is rate limited
var ErrRateLimited = errors.New("rate limited")

// ErrInvalidAuthHeader is returned when the auth header is invalid
var ErrInvalidAuthHeader = errors.New("invalid auth header format")

type errUserDoesNotHaveRequiredNFT struct {
	addresses []persist.Address
}

// JWTRequired is a middleware that requires a JWT to be present in the request
func JWTRequired(userRepository persist.UserRepository, ethClient *eth.Client, tokenIDs []persist.TokenID) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: ErrInvalidAuthHeader.Error()})
			return
		}
		authHeaders := strings.Split(header, " ")
		if len(authHeaders) == 2 {
			if authHeaders[0] == viper.GetString("ADMIN_PASS") {
				c.Set(UserIDContextKey, persist.DBID(authHeaders[1]))
				c.Next()
				return
			}
			if authHeaders[0] != "Bearer" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: ErrInvalidAuthHeader.Error()})
				return
			}
			// get string after "Bearer"
			jwt := authHeaders[1]
			// use an env variable as jwt secret as upposed to using a stateful secret stored in
			// database that is unique to every user and session
			valid, userID, err := authJwtParse(jwt, viper.GetString("JWT_SECRET"))
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: err.Error()})
				return
			}

			if !valid {
				c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: ErrInvalidJWT.Error()})
				return
			}

			if viper.GetBool("REQUIRE_NFTS") {
				user, err := userRepository.GetByID(c, userID)
				if err != nil {
					c.AbortWithStatusJSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
					return
				}
				has := false
				for _, addr := range user.Addresses {
					if res, _ := ethClient.HasNFTs(c, tokenIDs, addr); res {
						has = true
						break
					}
				}
				if !has {
					c.AbortWithStatusJSON(http.StatusBadRequest, util.ErrorResponse{Error: errUserDoesNotHaveRequiredNFT{addresses: user.Addresses}.Error()})
					return
				}
			}

			c.Set(UserIDContextKey, userID)
		} else {
			c.AbortWithStatusJSON(http.StatusBadRequest, util.ErrorResponse{Error: ErrInvalidAuthHeader.Error()})
			return
		}
		c.Next()
	}
}

// JWTOptional is a middleware that will detect if a JWT is present in the request
// and store the user id in the context if it is
func JWTOptional() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header != "" {
			authHeaders := strings.Split(header, " ")
			if len(authHeaders) == 2 {
				if authHeaders[0] == viper.GetString("ADMIN_PASS") {
					c.Set(UserIDContextKey, persist.DBID(authHeaders[1]))
					c.Next()
					return
				}
				// get string after "Bearer"
				jwt := authHeaders[1]
				valid, userID, _ := authJwtParse(jwt, viper.GetString("JWT_SECRET"))
				c.Set(AuthContextKey, valid)
				c.Set(UserIDContextKey, userID)
			} else {
				c.Set(AuthContextKey, false)
				c.Set(UserIDContextKey, persist.DBID(""))
			}
		} else {
			c.Set(AuthContextKey, false)
			c.Set(UserIDContextKey, persist.DBID(""))
		}
		c.Next()
	}
}

// RateLimited is a middleware that will rate limit requests based on IP address
func RateLimited() gin.HandlerFunc {
	return func(c *gin.Context) {
		limiter := rateLimiter.getLimiter(c.ClientIP())
		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusBadRequest, util.ErrorResponse{Error: ErrRateLimited.Error()})
			return
		}
		c.Next()
	}
}

// CORS is a middleware that will allow CORS requests
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestOrigin := c.Request.Header.Get("Origin")
		allowedOrigins := strings.Split(viper.GetString("ALLOWED_ORIGINS"), ",")

		if util.Contains(allowedOrigins, requestOrigin) || (strings.ToLower(viper.GetString("ENV")) == "development" && strings.HasPrefix(requestOrigin, "https://gallery-git-") && strings.HasSuffix(requestOrigin, "-gallery-so.vercel.app")) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", requestOrigin)
		}

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

// ErrLogger is a middleware that will log errors that are returned by requests
func ErrLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) > 0 {
			logrus.Errorf("%s %s %s %s %s", c.Request.Method, c.Request.URL, c.ClientIP(), c.Request.Header.Get("User-Agent"), c.Errors.JSON())
		}
	}
}

// GetUserIDFromCtx is a helper function that will return the user id from the context and panic if it does not exist
func GetUserIDFromCtx(c *gin.Context) persist.DBID {
	return c.MustGet(UserIDContextKey).(persist.DBID)
}

func (e errUserDoesNotHaveRequiredNFT) Error() string {
	return fmt.Sprintf("required tokens not owned by addresses: %v", e.addresses)
}
