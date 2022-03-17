package sentry

import (
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"

	"github.com/mikeydub/go-gallery/service/auth"
)

const (
	AuthSentryContextName  = "auth context"
	ErrorSentryContextName = "error context"
)

type SentryAuthContext struct {
	UserID        string
	Authenticated bool
	AuthError     error
}

type SentryErrorContext struct {
	Mapped     bool
	MappedTo   string
	StackIndex int
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
