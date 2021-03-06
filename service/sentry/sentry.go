package sentryutil

import (
	"context"
	"fmt"
	"strings"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"

	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/util"
)

const (
	authContextName  = "auth context"
	errorContextName = "error context"
	eventContextName = "event context"
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

type eventContext struct {
	ActorID   persist.DBID
	SubjectID persist.DBID
	Action    persist.Action
}

func ReportRemappedError(ctx context.Context, originalErr error, remappedErr interface{}) {
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		logger.For(ctx).Warnln("could not report error to Sentry because hub is nil")
		return
	}

	// Use a new scope so our error context and tag don't persist beyond this error
	hub.WithScope(func(scope *sentry.Scope) {
		if remappedErr != nil {
			SetErrorContext(scope, true, fmt.Sprintf("%T", remappedErr))
			scope.SetTag("remappedError", "true")
		} else {
			SetErrorContext(scope, false, "")
		}

		hub.CaptureException(originalErr)
	})
}

func ReportError(ctx context.Context, err error) {
	ReportRemappedError(ctx, err, nil)
}

func ScrubEventCookies(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	if event == nil || event.Request == nil {
		return event
	}

	var scrubbed []string
	for _, c := range strings.Split(event.Request.Cookies, "; ") {
		if strings.HasPrefix(c, auth.JWTCookieKey) {
			scrubbed = append(scrubbed, auth.JWTCookieKey+"=[filtered]")
		} else {
			scrubbed = append(scrubbed, c)
		}
	}
	cookies := strings.Join(scrubbed, "; ")

	event.Request.Cookies = cookies
	event.Request.Headers["Cookie"] = cookies
	return event
}

func ScrubEventHeaders(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	if event == nil || event.Request == nil {
		return event
	}

	scrubbed := map[string]string{}
	for k, v := range event.Request.Headers {
		if k == "Authorization" {
			scrubbed[k] = "[filtered]"
		} else {
			scrubbed[k] = v
		}
	}

	event.Request.Headers = scrubbed
	return event
}

func UpdateErrorFingerprints(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	if event == nil || hint == nil || hint.OriginalException == nil {
		return event
	}

	// This is a hacky way to do this -- we'd rather check the actual type than a string, but
	// the errors.errorString type isn't exported and we'd really like a way to separate those
	// errors on Sentry. It's not very useful to group every error created with errors.New().
	exceptionType := fmt.Sprintf("%T", hint.OriginalException)
	if exceptionType == "*errors.errorString" {
		event.Fingerprint = []string{"{{ default }}", hint.OriginalException.Error()}
	}

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

func SetEventContext(scope *sentry.Scope, actorID, subjectID persist.DBID, action persist.Action) {
	eventCtx := eventContext{
		ActorID:   actorID,
		SubjectID: subjectID,
		Action:    action,
	}

	scope.SetContext(eventContextName, eventCtx)
}

func NewSentryHubGinContext(ctx context.Context) *gin.Context {
	cpy := util.GinContextFromContext(ctx).Copy()

	if hub := SentryHubFromContext(cpy); hub != nil {
		cpy.Request = cpy.Request.WithContext(sentry.SetHubOnContext(cpy.Request.Context(), hub.Clone()))
	}

	return cpy
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
