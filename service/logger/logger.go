package logger

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const loggerContextKey = "logger.logger"

var defaultLogger = logrus.New()
var defaultEntry = logrus.NewEntry(defaultLogger)

func InitLogger() {
	SetLoggerOptions(func(logger *logrus.Logger) {
		logger.SetReportCaller(true)

		if viper.GetString("ENV") != "production" {
			logger.SetLevel(logrus.DebugLevel)
		}

		if viper.GetString("ENV") == "local" {
			logger.SetFormatter(&logrus.TextFormatter{DisableQuote: true})
		} else {
			// Use a JSONFormatter for non-local environments because Google Cloud Logging works well with JSON-formatted log entries
			logger.SetFormatter(&logrus.JSONFormatter{})
		}
	})
}

func NewContextWithFields(parent context.Context, fields logrus.Fields) context.Context {
	return context.WithValue(parent, loggerContextKey, For(parent).WithFields(fields))
}

func SetLoggerOptions(optionsFunc func(logger *logrus.Logger)) {
	optionsFunc(defaultLogger)
}

func For(ctx context.Context) *logrus.Entry {
	if ctx == nil {
		return defaultEntry
	}

	// If ctx is a *gin.Context, get the underlying request context
	if gc, ok := ctx.(*gin.Context); ok {
		ctx = gc.Request.Context()
	}

	value := ctx.Value(loggerContextKey)
	if logger, ok := value.(*logrus.Entry); ok {
		return logger
	}

	return defaultEntry
}
