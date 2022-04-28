package sentry

import (
	"context"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"

	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/util"
)

type sentryContextKey string

const (
	AuthSentryContextName  = "auth context"
	ErrorSentryContextName = "error context"
	SentryHubContextKey    = sentryContextKey("sentryHub")
)

type SentryAuthContext struct {
	UserID        string
	Authenticated bool
	AuthError     error
}

type SentryErrorContext struct {
	Mapped   bool
	MappedTo string
}

func SetSentryAuthContext(gc *gin.Context, hub *sentry.Hub) {
	var authCtx SentryAuthContext
	var userCtx sentry.User

	if auth.GetUserAuthedFromCtx(gc) {
		userID := string(auth.GetUserIDFromCtx(gc))
		authCtx = SentryAuthContext{Authenticated: true, UserID: userID}
		userCtx = sentry.User{ID: userID}
	} else {
		authCtx = SentryAuthContext{AuthError: auth.GetAuthErrorFromCtx(gc)}
		userCtx = sentry.User{}
	}

	hub.Scope().SetContext(AuthSentryContextName, authCtx)
	hub.Scope().SetUser(userCtx)
}

func NewSentryHubContext(ctx context.Context, hub *sentry.Hub) context.Context {
	var cpy *sentry.Hub

	if hub != nil {
		cpy = hub.Clone()
	}

	return context.WithValue(ctx, SentryHubContextKey, cpy)
}

func SentryHubFromContext(ctx context.Context) *sentry.Hub {
	// Use request-scoped hub if available
	gc := util.GinContextFromContext(ctx)
	if hub := sentrygin.GetHubFromContext(gc); hub != nil {
		return hub
	}

	hub, ok := ctx.Value(SentryHubContextKey).(*sentry.Hub)
	if !ok {
		return nil
	}

	return hub
}
