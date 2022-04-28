package logger

import (
	"context"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

const loggerContextKey = "logger.logger"

var defaultLogger = logrus.NewEntry(logrus.StandardLogger())

func NewContextWithFields(parent context.Context, fields logrus.Fields) context.Context {
	return context.WithValue(parent, loggerContextKey, For(parent).WithFields(fields))
}

func NewContextWithSpan(parent context.Context, span *sentry.Span) context.Context {
	return NewContextWithFields(parent, logrus.Fields{
		"span-id":        span.SpanID,
		"parent-span-id": span.ParentSpanID,
	})
}

func For(ctx context.Context) *logrus.Entry {
	// If ctx is a *gin.Context, get the underlying request context
	if gc, ok := ctx.(*gin.Context); ok {
		ctx = gc.Request.Context()
	}

	value := ctx.Value(loggerContextKey)
	if logger, ok := value.(*logrus.Entry); ok {
		return logger
	}

	return NoCtx()
}

func NoCtx() *logrus.Entry {
	return defaultLogger
}
