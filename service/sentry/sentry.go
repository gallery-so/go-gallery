package sentryutil

import (
	"context"
	"strings"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"

	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/util"
)

const (
	authContextName  = "auth context"
	errorContextName = "error context"
)

type authContext struct {
	UserID        string
	Authenticated bool
	AuthError     error
}

type errorContext struct {
	Mapped   bool
	MappedTo string
}

func ScrubEventCookies(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event.Request == nil {
		return event
	}

	var scrubbed []string
	for _, c := range strings.Split(event.Request.Cookies, "; ") {
		if !strings.HasPrefix(c, auth.JWTCookieKey) {
			scrubbed = append(scrubbed, c)
		}
	}
	cookies := strings.Join(scrubbed, "; ")

	event.Request.Cookies = cookies
	event.Request.Headers["Cookie"] = cookies
	return event
}

func SetAuthContext(scope *sentry.Scope, gc *gin.Context) {
	var authCtx authContext
	var userCtx sentry.User

	if auth.GetUserAuthedFromCtx(gc) {
		userID := string(auth.GetUserIDFromCtx(gc))
		authCtx = authContext{Authenticated: true, UserID: userID}
		userCtx = sentry.User{ID: userID}
	} else {
		authCtx = authContext{AuthError: auth.GetAuthErrorFromCtx(gc)}
		userCtx = sentry.User{}
	}

	scope.SetContext(authContextName, authCtx)
	scope.SetUser(userCtx)
}

func SetErrorContext(scope *sentry.Scope, mapped bool, mappedTo string) {
	errCtx := errorContext{
		Mapped:   mapped,
		MappedTo: mappedTo,
	}

	scope.SetContext(errorContextName, errCtx)
}

func NewSentryHubContext(ctx context.Context, hub *sentry.Hub) context.Context {
	var cpy *sentry.Hub

	if hub != nil {
		cpy = hub.Clone()
	}

	return sentry.SetHubOnContext(ctx, cpy)
}

// SentryHubFromContext gets a Hub from the supplied context, or from an underlying
// gin.Context if one is available. NOTE: once gin 1.7.8 is released, this method can
// be removed in favor of sentry's default "sentry.GetHubFromContext" method, as gin 1.7.8
// will automatically check the request context for a value if it isn't found in the gin
// context.
func SentryHubFromContext(ctx context.Context) *sentry.Hub {
	// Get a hub via Sentry's standard mechanism if possible
	if hub := sentry.GetHubFromContext(ctx); hub != nil {
		return hub
	}

	// Otherwise, see if there's a hub stored on the gin context
	gc := util.GinContextFromContext(ctx)
	if hub := sentrygin.GetHubFromContext(gc); hub != nil {
		return hub
	}

	return nil
}
