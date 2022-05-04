package sentryutil

import (
	"context"
	"fmt"
	"github.com/mikeydub/go-gallery/service/logger"
	"net/http"
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
		if !strings.HasPrefix(c, auth.JWTCookieKey) {
			scrubbed = append(scrubbed, c)
		}
	}
	cookies := strings.Join(scrubbed, "; ")

	event.Request.Cookies = cookies
	event.Request.Headers["Cookie"] = cookies
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

type tracingTransport struct {
	http.RoundTripper

	continueOnly bool
}

// NewTracingTransport creates an http transport that will trace requests via Sentry. If continueOnly is true,
// traces will only be generated if they'd contribute to an existing parent trace (e.g. if a trace is not in progress,
// no new trace would be started).
func NewTracingTransport(roundTripper http.RoundTripper, continueOnly bool) *tracingTransport {
	// If roundTripper is already a tracer, grab its underlying RoundTripper instead
	if existingTracer, ok := roundTripper.(*tracingTransport); ok {
		return &tracingTransport{RoundTripper: existingTracer.RoundTripper, continueOnly: continueOnly}
	}

	return &tracingTransport{RoundTripper: roundTripper, continueOnly: continueOnly}
}

func (t *tracingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.continueOnly {
		transaction := sentry.TransactionFromContext(req.Context())
		if transaction == nil {
			return t.RoundTripper.RoundTrip(req)
		}
	}

	span := sentry.StartSpan(req.Context(), "http."+strings.ToLower(req.Method))
	span.Description = fmt.Sprintf("HTTP %s %s", req.Method, req.URL.String())
	defer span.Finish()

	// Send sentry-trace header in case the receiving service can continue our trace
	req.Header.Add("sentry-trace", span.TraceID.String())

	response, err := t.RoundTripper.RoundTrip(req)

	if span.Data == nil {
		span.Data = make(map[string]interface{})
	}
	span.Data["HTTP Status Code"] = response.StatusCode

	return response, err
}
