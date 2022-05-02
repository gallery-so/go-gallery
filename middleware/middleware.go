package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var rateLimiter = newIPRateLimiter(1, 5)

// ErrRateLimited is returned when the IP address has exceeded the rate limit
var ErrRateLimited = errors.New("rate limited")

var mixpanelDistinctIDs = map[string]string{}

type errUserDoesNotHaveRequiredNFT struct {
	addresses []persist.Wallet
}

// AuthRequired is a middleware that checks if the user is authenticated
func AuthRequired(userRepository persist.UserRepository, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		authHeaders := strings.Split(header, " ")
		if len(authHeaders) == 2 {
			if authHeaders[0] == viper.GetString("ADMIN_PASS") {
				auth.SetAuthStateForCtx(c, persist.DBID(authHeaders[1]), nil)
				c.Next()
				return
			}
		}
		jwt, err := c.Cookie(auth.JWTCookieKey)
		if err != nil {
			if err == http.ErrNoCookie {
				c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: auth.ErrNoCookie.Error()})
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: auth.ErrInvalidJWT.Error()})
			return
		}

		// use an env variable as jwt secret as upposed to using a stateful secret stored in
		// database that is unique to every user and session
		userID, err := auth.JWTParse(jwt, viper.GetString("JWT_SECRET"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: err.Error()})
			return
		}

		if viper.GetBool("REQUIRE_NFTS") {
			user, err := userRepository.GetByID(c, userID)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
				return
			}
			has := false
			for _, addr := range user.Wallets {
				if addr.Address.Chain != persist.ChainETH {
					continue
				}
				allowlist := auth.GetAllowlistContracts()
				for k, v := range allowlist {
					if res, _ := eth.HasNFTs(c, k, v, persist.EthereumAddress(addr.Address.AddressValue), ethClient); res {
						has = true
						break
					}
				}
			}
			if !has {
				c.AbortWithStatusJSON(http.StatusBadRequest, util.ErrorResponse{Error: errUserDoesNotHaveRequiredNFT{addresses: user.Wallets}.Error()})
				return
			}
		}

		auth.SetAuthStateForCtx(c, userID, nil)
		c.Next()
	}
}

// AuthOptional is a middleware that checks if the user is authenticated and if so stores
// auth data in the context
func AuthOptional() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		authHeaders := strings.Split(header, " ")
		if len(authHeaders) == 2 {
			if authHeaders[0] == viper.GetString("ADMIN_PASS") {
				auth.SetAuthStateForCtx(c, persist.DBID(authHeaders[1]), nil)
				c.Next()
				return
			}
		}
		jwt, err := c.Cookie(auth.JWTCookieKey)
		if err != nil {
			if err == http.ErrNoCookie {
				auth.SetAuthStateForCtx(c, "", err)
				c.Next()
				return
			}
			c.AbortWithError(http.StatusUnauthorized, err)
			return
		}
		userID, err := auth.JWTParse(jwt, viper.GetString("JWT_SECRET"))
		auth.SetAuthStateForCtx(c, userID, err)
		c.Next()
	}
}

// AddAuthToContext is a middleware that validates auth data and stores the results in the context
func AddAuthToContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		authHeaders := strings.Split(header, " ")
		if len(authHeaders) == 2 {
			if authHeaders[0] == viper.GetString("ADMIN_PASS") {
				auth.SetAuthStateForCtx(c, persist.DBID(authHeaders[1]), nil)
				c.Next()
				return
			}
		}

		jwt, err := c.Cookie(auth.JWTCookieKey)

		// Treat empty cookies the same way we treat missing cookies, since setting a cookie to the empty
		// string is how we "delete" them.
		if err == nil && jwt == "" {
			err = http.ErrNoCookie
		}

		if err != nil {
			if err == http.ErrNoCookie {
				err = auth.ErrNoCookie
			}

			auth.SetAuthStateForCtx(c, "", err)
			c.Next()
			return
		}

		userID, err := auth.JWTParse(jwt, viper.GetString("JWT_SECRET"))
		auth.SetAuthStateForCtx(c, userID, err)
		c.Next()
	}
}

// RateLimited is a middleware that rate limits requests by IP address
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

// HandleCORS sets the CORS headers
func HandleCORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestOrigin := c.Request.Header.Get("Origin")
		allowedOrigins := strings.Split(viper.GetString("ALLOWED_ORIGINS"), ",")

		if util.Contains(allowedOrigins, requestOrigin) || (util.Contains([]string{"development", "sandbox-backend"}, strings.ToLower(viper.GetString("ENV"))) && strings.HasSuffix(requestOrigin, "-gallery-so.vercel.app")) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", requestOrigin)
		}

		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, Set-Cookie, sentry-trace")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers, Content-Type, Set-Cookie")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// BlockRequest is a middleware that blocks posts from being created
func BlockRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusBadRequest, util.ErrorResponse{Error: "gallery in maintenance, please try again later"})
	}
}

// ErrLogger is a middleware that logs errors
func ErrLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) > 0 {
			logrus.Errorf("%s %s %s %s %s", c.Request.Method, c.Request.URL, c.ClientIP(), c.Request.Header.Get("User-Agent"), c.Errors.JSON())
		}
	}
}

// GinContextToContext is a middleware that adds the Gin context to the request context,
// allowing the Gin context to be retrieved from within GraphQL resolvers.
// See: https://gqlgen.com/recipes/gin/
func GinContextToContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), util.GinContextKey, c)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func (e errUserDoesNotHaveRequiredNFT) Error() string {
	return fmt.Sprintf("required tokens not owned by addresses: %v", e.addresses)
}
