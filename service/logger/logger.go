package logger

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const loggerContextKey = "logger.logger"

var defaultLogger = logrus.New()
var defaultEntry = logrus.NewEntry(defaultLogger)

// NewContextWithFields returns a new context with a log entry derived from the default logger.
func NewContextWithFields(parent context.Context, fields logrus.Fields) context.Context {
	return context.WithValue(parent, loggerContextKey, For(parent).WithFields(fields))
}

func SetLoggerOptions(optionsFunc func(logger *logrus.Logger)) {
	optionsFunc(defaultLogger)
}

// InitWithGCPDefaults initializes a logger suitable for most Google Cloud Logging purposes
func InitWithGCPDefaults() {
	SetLoggerOptions(func(logger *logrus.Logger) {
		logger.SetReportCaller(true)

		if viper.GetString("ENV") != "production" {
			logger.SetLevel(logrus.DebugLevel)
		}

		if viper.GetString("ENV") == "local" {
			logger.SetFormatter(&logrus.TextFormatter{DisableQuote: true})
		} else {
			// Use a GCPFormatter for Google Cloud Logging
			logger.SetFormatter(NewGCPFormatter())
		}
	})
}

// GCPFormatter is a logrus.JSONFormatter with additional handling to map log
// severity and timestamps to the specific named JSON fields ("severity" and "time")
// that Google Cloud Logging expects
type GCPFormatter struct {
	logrus.JSONFormatter
}

func NewGCPFormatter() *GCPFormatter {
	return &GCPFormatter{
		JSONFormatter: logrus.JSONFormatter{
			// GCP parses timestamps in RFC3339 format
			TimestampFormat: time.RFC3339Nano,

			// GCP expects log messages in a "message" field instead of the default logrus "msg" field
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyMsg: "message",
			},
		},
	}
}

type gcpLogSeverity string

// https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#logseverity
const (
	gcpSeverityDefault   gcpLogSeverity = "DEFAULT"
	gcpSeverityDebug     gcpLogSeverity = "DEBUG"
	gcpSeverityInfo      gcpLogSeverity = "INFO"
	gcpSeverityNotice    gcpLogSeverity = "NOTICE"
	gcpSeverityWarning   gcpLogSeverity = "WARNING"
	gcpSeverityError     gcpLogSeverity = "ERROR"
	gcpSeverityCritical  gcpLogSeverity = "CRITICAL"
	gcpSeverityAlert     gcpLogSeverity = "ALERT"
	gcpSeverityEmergency gcpLogSeverity = "EMERGENCY"
)

var logrusLevelToGCPSeverity = map[logrus.Level]gcpLogSeverity{
	logrus.DebugLevel: gcpSeverityDebug,
	logrus.InfoLevel:  gcpSeverityInfo,
	logrus.WarnLevel:  gcpSeverityWarning,
	logrus.ErrorLevel: gcpSeverityError,
	logrus.FatalLevel: gcpSeverityCritical,
	logrus.PanicLevel: gcpSeverityAlert,
}

func (f *GCPFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	// We can't use the JSONFormatter FieldMap to map logrus' levels to GCP levels,
	// because GCP doesn't have levels named "fatal" or "panic". Instead, we map
	// log levels manually to make sure all levels are accounted for.
	entry.Data["severity"] = logrusLevelToGCPSeverity[entry.Level]
	return f.JSONFormatter.Format(entry)
}

func For(ctx context.Context) *logrus.Entry {
	if ctx == nil {
		return defaultEntry.WithContext(nil)
	}

	// If ctx is a *gin.Context, get the underlying request context
	if gc, ok := ctx.(*gin.Context); ok {
		ctx = gc.Request.Context()
	}

	value := ctx.Value(loggerContextKey)
	if logger, ok := value.(*logrus.Entry); ok {
		return logger.WithContext(ctx)
	}

	return defaultEntry.WithContext(ctx)
}

// LoggedError wraps the original error and logging message.
type LoggedError struct {
	Message string         // The original message passed to the logger
	Err     error          // The error added to the logger
	Caller  *runtime.Frame // Available if logger is configured to report on the caller
}

func (e LoggedError) Error() string {
	msg := e.Message

	if e.Err != nil {
		msg += fmt.Sprintf(": %s", e.Err)
	}

	if e.Caller != nil {
		msg += fmt.Sprintf("; occurred around: %s:%s %d",
			e.Caller.File, e.Caller.Function, e.Caller.Line,
		)
	}

	return msg
}

// GinFormatter returns a gin.LogFormatter that includes additional context via logrus
func GinFormatter() gin.LogFormatter {
	// Wrapping gin logs with logrus is noisy in a local development console, so only do it for
	// cloud logs (which will handle JSON logs gracefully)
	wrapWithLogrus := true
	if viper.GetString("ENV") == "local" {
		wrapWithLogrus = false
	}

	return func(param gin.LogFormatterParams) string {
		// Gin's default logger, copy/pasted from gin/logger.go
		defaultLogFormatter := func(param gin.LogFormatterParams) string {
			var statusColor, methodColor, resetColor string
			if param.IsOutputColor() {
				statusColor = param.StatusCodeColor()
				methodColor = param.MethodColor()
				resetColor = param.ResetColor()
			}

			if param.Latency > time.Minute {
				param.Latency = param.Latency.Truncate(time.Second)
			}
			return fmt.Sprintf("[GIN] %v |%s %3d %s| %13v | %15s |%s %-7s %s %#v\n%s",
				param.TimeStamp.Format("2006/01/02 - 15:04:05"),
				statusColor, param.StatusCode, resetColor,
				param.Latency,
				param.ClientIP,
				methodColor, param.Method, resetColor,
				param.Path,
				param.ErrorMessage,
			)
		}

		// Custom handling to output gin's log statements with extra context via logrus
		str := defaultLogFormatter(param)
		if wrapWithLogrus && param.Request.Context() != nil {
			logEntry := For(param.Request.Context())

			// Fill in known httpRequest fields for GCP.
			// See: https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#HttpRequest
			logEntry = logEntry.WithFields(logrus.Fields{
				"httpRequest": map[string]interface{}{
					"requestMethod": param.Method,
					"requestUrl":    param.Request.URL,
					"status":        param.StatusCode,
					"responseSize":  param.BodySize,
					"latency":       param.Latency,
					"remoteIp":      param.ClientIP,
					"userAgent":     param.Request.UserAgent(),
					"protocol":      param.Request.Proto,
					"referer":       param.Request.Referer(),
				},
			})

			if logEntry.Time.IsZero() {
				logEntry.Time = time.Now()
			}

			if param.StatusCode >= http.StatusOK && param.StatusCode < http.StatusMultipleChoices {
				logEntry.Level = logrus.InfoLevel
			} else {
				logEntry.Level = logrus.WarnLevel
			}

			logEntry.Message = str

			// Use the logrus logEntry to format the output, and then return the string back to gin
			// so it can write the message
			if output, err := logEntry.String(); err == nil {
				return output
			}
		}

		return str
	}
}
