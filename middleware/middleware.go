package middleware

import (
	"context"
	"fmt"
	"github.com/mikeydub/go-gallery/service/auth/basicauth"
	"net/http"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/sirupsen/logrus"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/util"
)

var mixpanelDistinctIDs = map[string]string{}

type errBadTaskRequest struct {
	msg string
}

func (e errBadTaskRequest) Error() string {
	return fmt.Sprintf("bad task request: %s", e.msg)
}

// AdminRequired is a middleware that checks if the user is authenticated as an admin
func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") != env.GetString("ADMIN_PASS") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, util.ErrorResponse{Error: "Unauthorized"})
			return
		}
		c.Next()
	}
}

type BasicAuthOptionBuilder struct{}

func (BasicAuthOptionBuilder) WithFailureStatus(statusCode int) BasicAuthOption {
	return func(o *basicAuthOptions) {
		o.failureStatus = statusCode
	}
}

func (BasicAuthOptionBuilder) WithUsername(username string) BasicAuthOption {
	return func(o *basicAuthOptions) {
		o.username = &username
	}
}

type BasicAuthOption func(*basicAuthOptions)

type basicAuthOptions struct {
	username      *string
	failureStatus int
}

// BasicHeaderAuthRequired is a middleware that checks if the request has a Basic Auth header matching
// the specified password. A username can optionally be specified via WithUsername. Failures return
// http.StatusUnauthorized by default, but this can be changed via WithFailureStatus (for example,
// returning a 200 to Cloud Tasks to indicate that the task shouldn't be retried). Failures will
// always abort the request, regardless of the failure status code returned.
func BasicHeaderAuthRequired(password string, options ...BasicAuthOption) gin.HandlerFunc {
	o := &basicAuthOptions{
		username:      nil,
		failureStatus: http.StatusUnauthorized,
	}

	for _, opt := range options {
		opt(o)
	}

	return func(c *gin.Context) {
		if !basicauth.AuthorizeHeader(c, o.username, password) {
			c.AbortWithStatusJSON(o.failureStatus, util.ErrorResponse{Error: "Unauthorized"})
			return
		}

		c.Next()
	}
}

// TaskRequired checks that the request comes from Cloud Tasks.
// Returns a 200 status to remove the message from the queue if it is a bad request.
func TaskRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskName := c.Request.Header.Get("X-CloudTasks-TaskName")
		if taskName == "" {
			c.AbortWithError(http.StatusOK, errBadTaskRequest{"invalid task"})
			return
		}

		queueName := c.Request.Header.Get("X-CloudTasks-QueueName")
		if queueName == "" {
			c.AbortWithError(http.StatusOK, errBadTaskRequest{"invalid queue"})
			return
		}

		c.Next()
	}
}

// AddAuthToContext is a middleware that validates auth data and stores the results in the context
func AddAuthToContext() gin.HandlerFunc {
	return func(c *gin.Context) {
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

		userID, err := auth.JWTParse(jwt, env.GetString("JWT_SECRET"))
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
		hub.Scope().AddEventProcessor(auth.ScrubEventCookies)

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

		if c.Request.Method == "OPTIONS" {
			// Don't sample OPTIONS requests; there's nothing to trace and they eat up our Sentry quota.
			// Using a sampling decision here (instead of simply omitting the span) ensures that any
			// child spans will also be filtered out.
			span.Sampled = sentry.SampledFalse
		}

		defer tracing.FinishSpan(span)

		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
