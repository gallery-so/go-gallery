package sentryutil

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"

	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/mikeydub/go-gallery/util"
)

const (
	errorContextName  = "error context"
	eventContextName  = "event context"
	loggerContextName = "logger context"
	TokenContextName  = "NFT context" // Sentry excludes contexts that contain "token" so we use "NFT" instead
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

// ReportRemappedError reports an error with additional context indicating the original error and the remapped error.
func ReportRemappedError(ctx context.Context, originalErr error, remappedErr interface{}) {
	reportError(ctx, originalErr, func(scope *sentry.Scope) {
		if remappedErr != nil {
			SetErrorContext(scope, true, fmt.Sprintf("%T", remappedErr))
			scope.SetTag("remappedError", "true")
		} else {
			SetErrorContext(scope, false, "")
		}
	})
}

// ReportTokenError reports an error that occurred while processing a token.
func ReportTokenError(ctx context.Context, err error, runID persist.DBID, chain persist.Chain, contractAddress persist.Address, tokenID persist.TokenID, isSpam bool) {
	reportError(ctx, err, func(scope *sentry.Scope) {
		SetRunTags(scope, runID)
		SetTokenTags(scope, chain, contractAddress, tokenID)
		SetTokenContext(scope, chain, contractAddress, tokenID, isSpam)
	})
}

// ReportError reports an error to Sentry as-is.
func ReportError(ctx context.Context, err error) {
	reportError(ctx, err)
}

// reportError add any additional information to the scope before reporting the error to Sentry.
func reportError(ctx context.Context, err error, scopeFuncs ...func(*sentry.Scope)) {
	hub := SentryHubFromContext(ctx)
	if hub == nil {
		logger.For(ctx).Warnln("could not report error to Sentry because hub is nil")
		return
	}

	hub.WithScope(func(scope *sentry.Scope) {
		for _, f := range scopeFuncs {
			f(scope)
		}
		hub.CaptureException(err)
	})
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

	if isErrErrorString(hint.OriginalException) {
		event.Fingerprint = []string{"{{ default }}", hint.OriginalException.Error()}
	}

	return event
}

// UpdateLogErrorEvent updates the outgoing event with data from the logged error.
func UpdateLogErrorEvent(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	if wrapped, ok := hint.OriginalException.(logger.LoggedError); ok {
		if wrapped.Err != nil {
			if isErrErrorString(wrapped.Err) {
				// Group by the actual error string so those errors are better categorized on Sentry.
				event.Fingerprint = []string{"{{ type }}", wrapped.Err.Error()}
			} else {
				// Group first by the LoggedError type and further group by the actual wrapped error.
				event.Fingerprint = []string{"{{ type }}", fmt.Sprintf("%T", wrapped.Err)}
			}
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

func SetErrorContext(scope *sentry.Scope, mapped bool, mappedTo string) {
	errCtx := sentry.Context{
		"Mapped":   mapped,
		"MappedTo": mappedTo,
	}

	scope.SetContext(errorContextName, errCtx)
}

func SetEventContext(scope *sentry.Scope, actorID, subjectID persist.DBID, action persist.Action) {
	eventCtx := sentry.Context{
		"ActorID":   actorID,
		"SubjectID": subjectID,
		"Action":    action,
	}

	scope.SetContext(eventContextName, eventCtx)
}

func SetTokenTags(scope *sentry.Scope, chain persist.Chain, contractAddress persist.Address, tokenID persist.TokenID) {
	scope.SetTag("chain", fmt.Sprintf("%d", chain))
	scope.SetTag("contractAddress", contractAddress.String())
	scope.SetTag("nftID", string(tokenID))
	assetPage := assetURL(chain, contractAddress, tokenID)
	if len(assetPage) > 200 {
		assetPage = "see token context"
	}
	scope.SetTag("assetURL", assetPage)
}

func assetURL(chain persist.Chain, contractAddress persist.Address, tokenID persist.TokenID) string {
	switch chain {
	case persist.ChainETH:
		return fmt.Sprintf("https://opensea.io/assets/%s/%d", contractAddress.String(), tokenID.ToInt())
	case persist.ChainTezos:
		return fmt.Sprintf("https://objkt.com/asset/%s/%d", contractAddress.String(), tokenID.ToInt())
	default:
		return ""
	}
}

func SetTokenContext(scope *sentry.Scope, chain persist.Chain, contractAddress persist.Address, tokenID persist.TokenID, isSpam bool) {
	scope.SetContext(TokenContextName, sentry.Context{
		"Chain":           chain,
		"ContractAddress": contractAddress,
		"NftID":           tokenID, // Sentry strips fields containing 'token'
		"IsSpam":          isSpam,
		"AssetURL":        assetURL(chain, contractAddress, tokenID),
	})
}

func SetRunTags(scope *sentry.Scope, runID persist.DBID) {
	scope.SetTag("runID", runID.String())
	scope.SetTag("log", "go/tp-runs/"+runID.String())
}

// NewSentryHubGinContext returns a new Gin context with a cloned hub of the original context's hub.
// The hub is added to the context's request so that the sentrygin middleware is able to find it.
func NewSentryHubGinContext(ctx context.Context) *gin.Context {
	cpy := util.MustGetGinContext(ctx).Copy()

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
// gin.Context if one is available.
func SentryHubFromContext(ctx context.Context) *sentry.Hub {
	// Get a hub via Sentry's standard mechanism if possible
	if hub := sentry.GetHubFromContext(ctx); hub != nil {
		return hub
	}

	// Otherwise, see if there's a hub stored on the gin context
	if gc := util.GetGinContext(ctx); gc != nil {
		if hub := sentrygin.GetHubFromContext(gc); hub != nil {
			return hub
		}

		// If not, try to get it from the request context
		if hub := sentry.GetHubFromContext(gc.Request.Context()); hub != nil {
			return hub
		}
	}

	return nil
}

// sentryLoggerHook reports messages to Sentry.
type sentryLoggerHook struct {
	crumbTrailLimit int
	reportLevels    []logrus.Level
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

func getSpanDuration(s *sentry.Span) time.Duration {
	return s.EndTime.Sub(s.StartTime)
}

// Sentry uses milliseconds for its trace fields, and it keeps things consistent if we do it too
func durationToMsFloat(duration time.Duration) float64 {
	return float64(duration.Microseconds()) / 1000.0
}

// SpanFilterEventProcessor applies a progressive filter to spans, removing the shortest spans until the total span count
// is less than the specified maxSpans value. Initially, spans shorter than minSpanDuration will be dropped, but each filter
// pass (up to maxFilterPasses) will double the minSpanDuration until enough spans have been filtered out.
// If alwaysFilter is specified, spans shorter than minSpanDuration will be removed, even if the total span count is low
// enough not to require any filtering.
func SpanFilterEventProcessor(ctx context.Context, maxSpans int, minSpanDuration time.Duration, maxFilterPasses int, alwaysFilter bool) sentry.EventProcessor {
	return func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
		if event == nil || event.Type != "transaction" || len(event.Spans) == 0 {
			return event
		}

		type rollup struct {
			Name     string
			RawSpans []*sentry.Span
			Children map[string]*rollup
		}

		type spanData struct {
			Parent        *spanData
			RawSpan       *sentry.Span
			IsFiltered    bool
			RollupsByName map[string]*rollup
			Duration      time.Duration
		}

		// Record how long the filtering process takes
		spanFilterStartTime := time.Now()

		spanDataBySpanID := make(map[sentry.SpanID]*spanData)
		spans := make([]*spanData, 0, len(event.Spans))

		for _, span := range event.Spans {
			if span != nil {
				sd := &spanData{RawSpan: span}
				spanDataBySpanID[span.SpanID] = sd
				spans = append(spans, sd)
			}
		}

		for _, span := range spans {
			span.Parent = spanDataBySpanID[span.RawSpan.ParentSpanID]
			span.Duration = getSpanDuration(span.RawSpan)
		}

		// Propagate span times from child to parent.
		// Use depth counter to avoid infinite looping if a cycle is encountered.
		for _, span := range spans {
			for depth := 0; depth < 1000; depth++ {
				if span.Parent == nil {
					break
				}

				child := span.RawSpan
				parent := span.Parent.RawSpan
				updatedParent := false

				if child.EndTime.After(parent.EndTime) {
					parent.EndTime = child.EndTime
					updatedParent = true
				}

				// This generally shouldn't happen, but if it does, we still want the parent span to fully encapsulate its children
				if child.StartTime.Before(parent.StartTime) {
					logger.For(ctx).Warnf("child span '%s - %s' started at %v, before parent span '%s - %s' started at %v\n",
						child.Op, child.Description, child.StartTime, parent.Op, parent.Description, parent.StartTime)

					parent.StartTime = child.StartTime
					updatedParent = true
				}

				if updatedParent {
					// If the parent span has been updated, we should recalculate its duration to see if we should keep it
					span.Parent.Duration = getSpanDuration(span.Parent.RawSpan)
				}

				span = span.Parent
			}
		}

		var filteredSpans []*spanData

		reportedFilterPasses := 0
		reportedMinSpanDuration := time.Duration(0)

		// Keep filtering and doubling the minSpanDuration until we've filtered out enough spans
		for filterPass := 1; filterPass <= maxFilterPasses; filterPass++ {
			if (!alwaysFilter || filterPass > 1) && len(spans) <= maxSpans {
				break
			}

			logger.For(ctx).Infof("span filter pass %d, %d spans, minDurationForPass: %v\n", filterPass, len(spans), minSpanDuration)

			allowedSpans := spans[:0]
			for _, span := range spans {
				// Spans without parents are always allowed
				if span.Parent == nil || span.Duration > minSpanDuration {
					allowedSpans = append(allowedSpans, span)
				} else {
					filteredSpans = append(filteredSpans, span)
					span.IsFiltered = true
				}
			}

			reportedFilterPasses = filterPass
			reportedMinSpanDuration = minSpanDuration

			spans = allowedSpans
			minSpanDuration *= 2
		}

		var filterPath []*spanData

		// Roll filtered spans up to their nearest unfiltered ancestor
		for _, span := range filteredSpans {
			filterPath = append(filterPath[:0], span)
			span = span.Parent // will be non-nil, since filtered spans must have parents

			// As above, use a depth counter to break cycles instead of infinite looping
			for depth := 0; depth < 1000; depth++ {
				if span.IsFiltered {
					filterPath = append(filterPath, span)
					span = span.Parent
					continue
				}

				// We've reached the nearest allowed ancestor! Lazy initialize it.
				if span.RollupsByName == nil {
					span.RollupsByName = make(map[string]*rollup)
				}

				rollupsByName := span.RollupsByName

				// Iterate backward so we're going from parent -> child
				for i := len(filterPath) - 1; i >= 0; i-- {
					filteredSpan := filterPath[i]
					rawSpan := filteredSpan.RawSpan

					var currentRollup *rollup
					var ok bool

					if currentRollup, ok = rollupsByName[rawSpan.Description]; !ok {
						currentRollup = &rollup{Name: rawSpan.Description}
						rollupsByName[rawSpan.Description] = currentRollup
					}

					// Only add the child span that's actively being rolled up; its parent spans (if any) will be
					// added during their own roll-up step
					if i == 0 {
						currentRollup.RawSpans = append(currentRollup.RawSpans, rawSpan)
					}

					// If we haven't reached the final child span yet, make sure there's a map for the next
					// level of rollup depth
					if currentRollup.Children == nil && i > 0 {
						currentRollup.Children = make(map[string]*rollup)
					}

					rollupsByName = currentRollup.Children
				}

				break
			}
		}

		var getRollupStats func(*spanData, map[string]*rollup, map[string]interface{}) (time.Time, time.Time)

		getRollupStats = func(rollupTo *spanData, rollupsByName map[string]*rollup, stats map[string]interface{}) (time.Time, time.Time) {
			var rollupStart time.Time
			var rollupEnd time.Time

			// Get any child span to initialize the start and end times for the whole rollup
			for _, r := range rollupsByName {
				rollupStart = r.RawSpans[0].StartTime
				rollupEnd = r.RawSpans[0].EndTime
				break
			}

			// For every group of rolled up spans, figure out their containing interval (earliest start time to
			// latest end time) and the average time per span.
			for name, r := range rollupsByName {
				intervalStart := r.RawSpans[0].StartTime
				intervalEnd := r.RawSpans[0].EndTime
				cumulativeDuration := time.Duration(0)

				for _, s := range r.RawSpans {
					if s.StartTime.Before(intervalStart) {
						intervalStart = s.StartTime
					}

					if s.EndTime.After(intervalEnd) {
						intervalEnd = s.EndTime
					}

					cumulativeDuration += getSpanDuration(s)
				}

				avgDuration := time.Duration(int64(cumulativeDuration) / int64(len(r.RawSpans)))

				// Record interval start/end times relative to the parent span these are being rolled up to
				relativeStart := intervalStart.Sub(rollupTo.RawSpan.StartTime)
				relativeEnd := intervalEnd.Sub(rollupTo.RawSpan.StartTime)

				statsForName := fmt.Sprintf("%s (%d) | %.3fms avg | [%.3fms - %.3fms] range", name, len(r.RawSpans),
					durationToMsFloat(avgDuration), durationToMsFloat(relativeStart), durationToMsFloat(relativeEnd))

				childStats := make(map[string]interface{})
				stats[statsForName] = childStats

				if r.Children != nil {
					getRollupStats(rollupTo, r.Children, childStats)
				}

				if intervalStart.Before(rollupStart) {
					rollupStart = intervalStart
				}

				if intervalEnd.After(rollupEnd) {
					rollupEnd = intervalEnd
				}
			}

			return rollupStart, rollupEnd
		}

		numUnfilteredSpans := len(event.Spans)
		event.Spans = event.Spans[:0]

		for _, span := range spans {
			if span.RollupsByName != nil {
				filteredSpanStats := make(map[string]interface{})
				rollupStart, rollupEnd := getRollupStats(span, span.RollupsByName, filteredSpanStats)

				if span.RawSpan.Data == nil {
					span.RawSpan.Data = make(map[string]interface{})
				}

				data := span.RawSpan.Data
				data["Filtered Spans"] = filteredSpanStats
				data["Filtered Span Range"] = fmt.Sprintf("[%.3fms - %.3fms]", durationToMsFloat(rollupStart.Sub(span.RawSpan.StartTime)), durationToMsFloat(rollupEnd.Sub(span.RawSpan.StartTime)))
			}

			event.Spans = append(event.Spans, span.RawSpan)
		}

		timeTaken := time.Since(spanFilterStartTime)
		filteredFrom := numUnfilteredSpans
		filteredTo := len(event.Spans)
		numDropped := 0

		if filteredFrom != filteredTo {
			logger.For(ctx).Infof("filtered %d spans down to %d spans in %v\n", filteredFrom, filteredTo, timeTaken)
		}

		// If we still have too many spans after filtering, we need to drop some
		if len(event.Spans) > maxSpans {
			numDropped = len(event.Spans) - maxSpans
			logger.For(ctx).Warnf("dropping %d spans to reduce total from %d to %d\n", numDropped, len(event.Spans), maxSpans)
			event.Spans = event.Spans[:maxSpans]
		}

		// Add filtering metadata to the event
		event.Contexts["Span Filtering"] = map[string]interface{}{
			"Filtering Took":    fmt.Sprintf("%.3fms", durationToMsFloat(timeTaken)),
			"Filtered From":     fmt.Sprintf("%d spans", filteredFrom),
			"Filtered To":       fmt.Sprintf("%d spans", filteredTo),
			"Dropped":           fmt.Sprintf("%d spans", numDropped),
			"Min Span Duration": fmt.Sprintf("%.3fms", durationToMsFloat(reportedMinSpanDuration)),
			"Filter Passes":     reportedFilterPasses,
		}

		return event
	}
}

// This is a hacky way to do this -- we'd rather check the actual type than a string, but
// the errors.errorString type isn't exported and we'd really like a way to separate those
// errors on Sentry. It's not very useful to group every error created with errors.New().
func isErrErrorString(err error) bool {
	if fmt.Sprintf("%T", err) == "*errors.errorString" {
		return true
	}
	return false
}
