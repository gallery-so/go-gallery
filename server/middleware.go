package server

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
	userIDcontextKey = "user_id"
	authContextKey   = "authenticated"
)

var rateLimiter = newIPRateLimiter(1, 5)

var errInvalidJWT = errors.New("invalid JWT")

var errRateLimited = errors.New("rate limited")

var errInvalidAuthHeader = errors.New("invalid auth header format")

type errUserDoesNotHaveRequiredNFT struct {
	addresses []string
}

func jwtRequired(userRepository persist.UserRepository, ethClient *eth.Client, tokenIDs []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: errInvalidAuthHeader.Error()})
			return
		}
		authHeaders := strings.Split(header, " ")
		if len(authHeaders) == 2 {
			if authHeaders[0] != "Bearer" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: errInvalidAuthHeader.Error()})
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
				c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: errInvalidJWT.Error()})
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

			c.Set(userIDcontextKey, userID)
		} else {
			c.AbortWithStatusJSON(http.StatusBadRequest, util.ErrorResponse{Error: errInvalidAuthHeader.Error()})
			return
		}
		c.Next()
	}
}

func jwtOptional() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header != "" {
			authHeaders := strings.Split(header, " ")
			if len(authHeaders) == 2 {
				// get string after "Bearer"
				jwt := authHeaders[1]
				valid, userID, _ := authJwtParse(jwt, viper.GetString("JWT_SECRET"))
				c.Set(authContextKey, valid)
				c.Set(userIDcontextKey, userID)
			} else {
				c.Set(authContextKey, false)
				c.Set(userIDcontextKey, persist.DBID(""))
			}
		} else {
			c.Set(authContextKey, false)
			c.Set(userIDcontextKey, persist.DBID(""))
		}
		c.Next()
	}
}

func rateLimited() gin.HandlerFunc {
	return func(c *gin.Context) {
		limiter := rateLimiter.getLimiter(c.ClientIP())
		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusBadRequest, util.ErrorResponse{Error: errRateLimited.Error()})
			return
		}
		c.Next()
	}
}

func handleCORS() gin.HandlerFunc {
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

func errLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) > 0 {
			logrus.Errorf("%s %s %s %s %s", c.Request.Method, c.Request.URL, c.ClientIP(), c.Request.Header.Get("User-Agent"), c.Errors.JSON())
		}
	}
}

func getUserIDfromCtx(c *gin.Context) persist.DBID {
	return c.MustGet(userIDcontextKey).(persist.DBID)
}

func (e errUserDoesNotHaveRequiredNFT) Error() string {
	return fmt.Sprintf("required tokens not owned by addresses: %v", e.addresses)
}
