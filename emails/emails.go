package emails

import (
	"context"
	htmltemplate "html/template"
	"net/http"
	plaintemplate "text/template"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// InitServer initializes the mediaprocessing server
func InitServer() {
	router := coreInitServer()
	logger.For(nil).Info("Starting tokenprocessing server...")
	http.Handle("/", router)
}

func coreInitServer() *gin.Engine {
	ctx := configureRootContext()

	setDefaults()
	initSentry()
	initLogger()

	t, err := htmltemplate.ParseGlob("./emails/templates/html/*")
	if err != nil {
		logger.For(ctx).Fatalf("failed to parse templates: %s", err)
	}

	pt, err := plaintemplate.ParseGlob("./emails/templates/plain/*")
	if err != nil {
		logger.For(ctx).Fatalf("failed to parse templates: %s", err)
	}

	pgxClient := postgres.NewPgxClient()

	queries := coredb.New(pgxClient)

	loaders := dataloader.NewLoaders(ctx, queries, false)

	client := sendgrid.NewSendClient(viper.GetString("SENDGRID_API_KEY"))

	http.DefaultClient = &http.Client{Transport: tracing.NewTracingTransport(http.DefaultTransport, false)}

	router := gin.Default()

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logger.For(ctx).Info("Registering handlers...")

	return handlersInitServer(router, loaders, queries, client, t, pt)
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("VERSION", "")
	viper.SetDefault("SENDGRID_API_KEY", "")
	viper.SetDefault("FROM_EMAIL", "test@gallery.so")
	viper.SetDefault("SENDGRID_DEFAULT_LIST_ID", "c63e40ab-5049-4ce1-9d14-8742a3c5c1a8")

	viper.AutomaticEnv()

	if viper.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
	} else {
		envFile := util.ResolveEnvFile("emails")
		util.LoadEnvFile(envFile)
	}

	if viper.GetString("ENV") != "local" {
		util.EnvVarMustExist("SENTRY_DSN", "")
		util.EnvVarMustExist("VERSION", "")
	}
}

func newThrottler() *throttle.Locker {
	return throttle.NewThrottleLocker(redis.NewCache(redis.TokenProcessingThrottleDB), time.Minute*5)
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
		Release:          viper.GetString("VERSION"),
		AttachStacktrace: true,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			event = sentryutil.ScrubEventCookies(event, hint)
			event = sentryutil.UpdateErrorFingerprints(event, hint)
			event = sentryutil.UpdateLogErrorEvent(event, hint)
			return event
		},
	})

	if err != nil {
		logger.For(nil).Fatalf("failed to start sentry: %s", err)
	}
}

func initLogger() {
	logger.SetLoggerOptions(func(l *logrus.Logger) {
		l.SetReportCaller(true)

		if viper.GetString("ENV") != "production" {
			l.SetLevel(logrus.DebugLevel)
		}

		if viper.GetString("ENV") == "local" {
			l.SetFormatter(&logrus.TextFormatter{DisableQuote: true})
		} else {
			// Use a JSONFormatter for non-local environments because Google Cloud Logging works well with JSON-formatted log entries
			l.SetFormatter(&logrus.JSONFormatter{})
		}

	})
}

// configureRootContext configures the main context from which other contexts are derived.
func configureRootContext() context.Context {
	ctx := logger.NewContextWithLogger(context.Background(), logrus.Fields{}, logrus.New())
	logger.For(ctx).Logger.SetReportCaller(true)
	logger.For(ctx).Logger.AddHook(sentryutil.SentryLoggerHook)
	return sentry.SetHubOnContext(ctx, sentry.CurrentHub())
}
