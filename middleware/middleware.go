package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/sirupsen/logrus"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/mikeydub/go-gallery/service/logger"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/tracing"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

var mixpanelDistinctIDs = map[string]string{}

type errUserDoesNotHaveRequiredNFT struct {
	addresses []persist.Wallet
}

// AuthRequired is a middleware that checks if the user is authenticated
func AuthRequired(userRepository postgres.UserRepository, ethClient *ethclient.Client) gin.HandlerFunc {
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

		// If we have a successfully authenticated user, add their ID to all subsequent logging
		if err == nil {
			loggerCtx := logger.NewContextWithFields(c.Request.Context(), logrus.Fields{
				"authedUserId": userID,
			})
			c.Request = c.Request.WithContext(loggerCtx)
		}

		c.Next()
	}
}

// RateLimited is a middleware that rate limits requests by IP address
func RateLimited(lim *KeyRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		canContinue, tryAgainAfter, err := lim.ForKey(c, c.ClientIP())
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		if !canContinue {
			c.AbortWithStatusJSON(http.StatusBadRequest, util.ErrorResponse{Error: fmt.Sprintf("rate limited, try again in %s", tryAgainAfter)})
			return
		}
		c.Next()
	}
}

// HandleCORS sets the CORS headers
func HandleCORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestOrigin := c.Request.Header.Get("Origin")

		if IsOriginAllowed(requestOrigin) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", requestOrigin)
		}

		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, Set-Cookie, sentry-trace, baggage")
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
			logger.For(c).Errorf("%s %s %s %s %s", c.Request.Method, c.Request.URL, c.ClientIP(), c.Request.Header.Get("User-Agent"), c.Errors.JSON())
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

func Sentry(reportGinErrors bool) gin.HandlerFunc {
	handler := sentrygin.New(sentrygin.Options{Repanic: true})

	return func(c *gin.Context) {
		// Clone a new hub for each request
		hub := sentry.CurrentHub().Clone()

		// We scrub JWT cookies from error events with a BeforeSend hook on our Sentry client, but
		// according to Sentry docs, BeforeSend isn't called for tracing transactions. Instead, we
		// have to use an event processor to scrub JWT cookies from transactions, so add one here.
		// See: https://develop.sentry.dev/sdk/performance/#interaction-with-beforesend-and-event-processors
		hub.Scope().AddEventProcessor(sentryutil.ScrubEventCookies)

		// Add the cloned hub to the request context so sentrygin will find it
		c.Request = c.Request.WithContext(sentry.SetHubOnContext(c.Request.Context(), hub))

		// Invoke the sentrygin handler. We don't call c.Next() here because sentrygin does it for us.
		handler(c)

		if reportGinErrors {
			for _, err := range c.Errors {
				sentryutil.ReportError(c.Request.Context(), err)
			}
		}
	}
}

func Tracing() gin.HandlerFunc {
	// Trace outgoing HTTP requests
	http.DefaultTransport = tracing.NewTracingTransport(http.DefaultTransport, true)
	http.DefaultClient = &http.Client{Transport: http.DefaultTransport}

	return func(c *gin.Context) {
		description := fmt.Sprintf("%s %s", c.Request.Method, c.Request.URL.Path)
		span, ctx := tracing.StartSpan(c.Request.Context(), "gin.server", description,
			sentry.TransactionName(description),
			sentry.ContinueFromRequest(c.Request),
		)

		defer tracing.FinishSpan(span)

		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

func (e errUserDoesNotHaveRequiredNFT) Error() string {
	return fmt.Sprintf("required tokens not owned by addresses: %v", e.addresses)
}
