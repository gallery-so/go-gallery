package sentryutil

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"

	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/util"
)

const (
	authContextName   = "auth context"
	errorContextName  = "error context"
	eventContextName  = "event context"
	loggerContextName = "logger context"
)

// SentryLoggerHook forwards log entries to Sentry.
var SentryLoggerHook = &sentryLoggerHook{crumbTrailLimit: sentryTrailLimit, reportLevels: logrus.AllLevels}
var logToSentryLevel = map[logrus.Level]sentry.Level{
	logrus.PanicLevel: sentry.LevelFatal,
	logrus.FatalLevel: sentry.LevelFatal,
	logrus.ErrorLevel: sentry.LevelError,
	logrus.WarnLevel:  sentry.LevelWarning,
	logrus.InfoLevel:  sentry.LevelInfo,
	logrus.DebugLevel: sentry.LevelDebug,
	logrus.TraceLevel: sentry.LevelDebug,
}
var sentryTrailLimit = 8

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

// UpdateLogErrorEvent updates the outgoing event with data from the logged error.
func UpdateLogErrorEvent(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	if wrapped, ok := hint.OriginalException.(logger.LoggedError); ok {
		if wrapped.Err != nil {
			event.Fingerprint = []string{"{{ default }}", wrapped.Err.Error()}
			mostRecent := len(event.Exception) - 1
			event.Exception[mostRecent].Type = reflect.TypeOf(wrapped.Err).String()

			// This really only works for errors created via the github.com/pkg/errors module.
			if newStack := sentry.ExtractStacktrace(wrapped.Err); newStack != nil {
				event.Exception[mostRecent].Stacktrace = newStack
			}
		}
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

// NewSentryHubGinContext returns a new Gin context with a cloned hub of the original context's hub.
// The hub is added to the context's request so that the sentrygin middleware is able to find it.
func NewSentryHubGinContext(ctx context.Context) *gin.Context {
	cpy := util.GinContextFromContext(ctx).Copy()

	if hub := SentryHubFromContext(cpy); hub != nil {
		cpy.Request = cpy.Request.WithContext(sentry.SetHubOnContext(cpy.Request.Context(), hub.Clone()))
	}

	return cpy
}

// NewSentryHubContext returns a copy of the parent context with an instance of its hub attached.
// If no hub exists, the default hub stored in the global namespace is used. This
// is useful for separating sentry-related data when starting new goroutines.
func NewSentryHubContext(ctx context.Context) context.Context {
	if hub := SentryHubFromContext(ctx); hub != nil {
		return sentry.SetHubOnContext(ctx, hub.Clone())
	}
	return sentry.SetHubOnContext(ctx, sentry.CurrentHub().Clone())
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

// sentryLoggerHook reports messages to Sentry.
type sentryLoggerHook struct {
	crumbTrailLimit int
	reportLevels    []logrus.Level
}

// SetSentryHookOptions configures the Sentry hook in the global namespace.
func SetSentryHookOptions(optionsFunc func(hook *sentryLoggerHook)) {
	optionsFunc(SentryLoggerHook)
}

// Levels returns the logging levels that the hook will fire on.
func (h sentryLoggerHook) Levels() []logrus.Level {
	return h.reportLevels
}

// Fire reports the log entry to Sentry.
func (h sentryLoggerHook) Fire(entry *logrus.Entry) error {
	if entry.Context == nil {
		return nil
	}
	if hub := SentryHubFromContext(entry.Context); hub != nil {
		switch isErr := entry.Level <= logrus.ErrorLevel; isErr {
		// Send as an error
		case true:
			if scope := hub.Scope(); scope == nil {
				hub.PushScope()
				defer hub.PopScope()
			}

			// Add logger fields as a context
			hub.Scope().SetContext(loggerContextName, entry.Data)

			if err, ok := entry.Data[logrus.ErrorKey].(error); ok {
				ReportError(entry.Context, logger.LoggedError{
					Message: entry.Message,
					Caller:  entry.Caller,
					Err:     err,
				})
			} else {
				ReportError(entry.Context, logger.LoggedError{
					Message: entry.Message,
					Caller:  entry.Caller,
				})
			}
		// Add to trail
		default:
			level := sentry.LevelDebug
			if sentryLevel, ok := logToSentryLevel[entry.Level]; !ok {
				level = sentryLevel
			}

			var category string
			if entry.Caller != nil {
				category = entry.Caller.Function
			}

			if scope := hub.Scope(); scope == nil {
				hub.PushScope()
			}

			hub.Scope().AddBreadcrumb(&sentry.Breadcrumb{
				Type:      "default",
				Category:  category,
				Level:     level,
				Message:   entry.Message,
				Data:      entry.Data,
				Timestamp: entry.Time,
			}, h.crumbTrailLimit)
		}
	}
	return nil
}

// RecoverAndRaise reports the panic to Sentry then re-raises it.
func RecoverAndRaise(ctx context.Context) {
	if err := recover(); err != nil {
		var hub *sentry.Hub

		if ctx != nil {
			hub = sentry.GetHubFromContext(ctx)
		}

		if hub == nil {
			hub = sentry.CurrentHub()
		}

		if hub == nil {
			panic(err)
		}

		defer sentry.Flush(2 * time.Second)
		hub.Recover(err)
		panic(err)
	}
}

// TransactionNameSafe sets the name for the current transaction if a name is not already set.
func TransactionNameSafe(name string) sentry.SpanOption {
	return func(s *sentry.Span) {
		hub := sentry.GetHubFromContext(s.Context())
		if hub == nil {
			hub = sentry.CurrentHub()
		}

		if hub.Scope().Transaction() != "" {
			return
		}

		sentry.TransactionName(name)(s)
	}
}
