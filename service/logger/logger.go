package logger

import (
	"context"
	"fmt"
	"runtime"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

const loggerContextKey = "logger.logger"

var defaultLogger = logrus.New()
var defaultEntry = logrus.NewEntry(defaultLogger)

func NewContextWithFields(parent context.Context, fields logrus.Fields) context.Context {
	return context.WithValue(parent, loggerContextKey, For(parent).WithFields(fields))
}

func SetLoggerOptions(optionsFunc func(logger *logrus.Logger)) {
	optionsFunc(defaultLogger)
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
