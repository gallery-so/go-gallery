package feed

import (
	"net/http"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/logger"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func Init() {
	setDefaults()

	initLogger()
	initSentry()

	router := coreInit()
	http.Handle("/", router)
}

func coreInit() *gin.Engine {
	logger.For(nil).Info("initializing server...")

	router := gin.Default()
	router.Use(middleware.ErrLogger(), middleware.Sentry(true), middleware.Tracing())

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
	}

	return handlersInit(router)
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("PORT", 4124)
	viper.SetDefault("FEED_SECRET", "feed-secret")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("TASK_QUEUE_HOST", "localhost:8123")
	viper.SetDefault("GCLOUD_FEEDBOT_TASK_QUEUE", "projects/gallery-local/locations/here/queues/feedbot")
	viper.SetDefault("FEEDBOT_SECRET", "feed-bot-secret")
	viper.AutomaticEnv()

	if viper.GetString("ENV") != "local" && viper.GetString("SENTRY_DSN") == "" {
		panic("SENTRY_DSN must be set")
	}
}

func initLogger() {
	logger.SetLoggerOptions(func(logger *logrus.Logger) {
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

func initSentry() {
	if viper.GetString("ENV") == "local" {
		logger.For(nil).Info("skipping sentry init")
		return
	}

	logger.For(nil).Info("initializing sentry...")

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              viper.GetString("SENTRY_DSN"),
		Environment:      viper.GetString("ENV"),
		TracesSampleRate: viper.GetFloat64("SENTRY_TRACES_SAMPLE_RATE"),
		AttachStacktrace: true,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			event = sentryutil.ScrubEventCookies(event, hint)
			event = sentryutil.UpdateErrorFingerprints(event, hint)
			return event
		},
	})

	if err != nil {
		logger.For(nil).Fatalf("failed to start sentry: %s", err)
	}
}
