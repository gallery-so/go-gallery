package logger

import (
	"context"
	"fmt"
	"github.com/spf13/viper"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

const loggerContextKey = "logger.logger"

var defaultLogger = logrus.New()
var defaultEntry = logrus.NewEntry(defaultLogger)

// NewContextWithFields returns a new context with a log entry derived from the default logger.
func NewContextWithFields(parent context.Context, fields logrus.Fields) context.Context {
	return context.WithValue(parent, loggerContextKey, For(parent).WithFields(fields))
}

// NewContextWithLogger returns a new context with a log entry derived from the input logger. This is useful
// when needing to configure the logger with options that differ from the default logger.
func NewContextWithLogger(parent context.Context, fields logrus.Fields, logger *logrus.Logger) context.Context {
	if logger == nil {
		return NewContextWithFields(parent, fields)
	}
	return context.WithValue(parent, loggerContextKey, logger.WithFields(fields))
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
			logger.SetFormatter(&GCPFormatter{})
		}
	})
}

// GCPFormatter is a logrus.JSONFormatter with additional handling to map log
// severity and timestamps to the specific named JSON fields ("severity" and "time")
// that Google Cloud Logging expects
type GCPFormatter struct {
	logrus.JSONFormatter
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
	entry.Data["severity"] = logrusLevelToGCPSeverity[entry.Level]
	entry.Data["time"] = entry.Time.Format(time.RFC3339Nano)
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
