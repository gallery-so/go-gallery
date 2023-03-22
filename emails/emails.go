package emails

import (
	"context"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/api/option"
)

// InitServer initializes the mediaprocessing server
func InitServer() {
	router := coreInitServer()
	logger.For(nil).Info("Starting emails server...")
	http.Handle("/", router)
}

func coreInitServer() *gin.Engine {
	setDefaults()
	initSentry()
	logger.InitWithGCPDefaults()

	pgxClient := postgres.NewPgxClient()

	queries := coredb.New(pgxClient)

	loaders := dataloader.NewLoaders(context.Background(), queries, false)

	sendgridClient := sendgrid.NewSendClient(env.GetString(context.Background(), "SENDGRID_API_KEY"))

	http.DefaultClient = &http.Client{Transport: tracing.NewTracingTransport(http.DefaultTransport, false)}

	router := gin.Default()

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())

	if env.GetString(context.Background(), "ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logger.For(nil).Info("Registering handlers...")

	var pub *pubsub.Client
	var err error
	if env.GetString(context.Background(), "ENV") == "local" {
		pub, err = pubsub.NewClient(context.Background(), env.GetString(context.Background(), "GOOGLE_CLOUD_PROJECT"), option.WithCredentialsJSON(util.LoadEncryptedServiceKey("./secrets/dev/service-key-dev.json")))
		if err != nil {
			panic(err)
		}
	} else {
		pub, err = pubsub.NewClient(context.Background(), env.GetString(context.Background(), "GOOGLE_CLOUD_PROJECT"))
		if err != nil {
			panic(err)
		}
	}

	go autoSendNotificationEmails(queries, sendgridClient, pub)

	redisClient := redis.NewClient(redis.EmailRateLimiterDB)

	return handlersInitServer(router, loaders, queries, sendgridClient, redisClient)
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("VERSION", "")
	viper.SetDefault("SENDGRID_API_KEY", "")
	viper.SetDefault("SENDGRID_VALIDATION_KEY", "")
	viper.SetDefault("FROM_EMAIL", "test@gallery.so")
	viper.SetDefault("SENDGRID_DEFAULT_LIST_ID", "865cea98-bf23-4ca3-a8d7-2dc9ea29951b")
	viper.SetDefault("SENDGRID_NOTIFICATIONS_TEMPLATE_ID", "d-6135d8f36e9946979b0dcf1800363ab4")
	viper.SetDefault("SENDGRID_VERIFICATION_TEMPLATE_ID", "d-b575d54dc86d40fdbf67b3119589475a")
	viper.SetDefault("SENDGRID_UNSUBSCRIBE_NOTIFICATIONS_GROUP_ID", 20676)
	viper.SetDefault("PUBSUB_NOTIFICATIONS_EMAILS_SUBSCRIPTION", "notifications-email-sub")
	viper.SetDefault("GOOGLE_CLOUD_PROJECT", "")
	viper.SetDefault("ADMIN_PASS", "admin")

	viper.AutomaticEnv()

	if env.GetString(context.Background(), "ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
	} else {
		fi := "local"
		if len(os.Args) > 0 {
			fi = os.Args[1]
		}
		envFile := util.ResolveEnvFile("emails", fi)
		util.LoadEncryptedEnvFile(envFile)
	}

	if env.GetString(context.Background(), "ENV") != "local" {
		util.VarNotSetTo("SENTRY_DSN", "")
		util.VarNotSetTo("VERSION", "")
		util.VarNotSetTo("SENDGRID_API_KEY", "")
		util.VarNotSetTo("JWT_SECRET", "")
		util.VarNotSetTo("FROM_EMAIL", "")
	}
}

func newThrottler() *throttle.Locker {
	return throttle.NewThrottleLocker(redis.NewCache(redis.EmailThrottleDB), time.Minute*5)
}

func initSentry() {
	if env.GetString(context.Background(), "ENV") == "local" {
		logger.For(nil).Info("skipping sentry init")
		return
	}

	logger.For(nil).Info("initializing sentry...")

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              env.GetString(context.Background(), "SENTRY_DSN"),
		Environment:      env.GetString(context.Background(), "ENV"),
		TracesSampleRate: env.Get[float64](context.Background(), "SENTRY_TRACES_SAMPLE_RATE"),
		Release:          env.GetString(context.Background(), "VERSION"),
		AttachStacktrace: true,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			event = auth.ScrubEventCookies(event, hint)
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

		if env.GetString(context.Background(), "ENV") != "production" {
			l.SetLevel(logrus.DebugLevel)
		}

		if env.GetString(context.Background(), "ENV") == "local" {
			l.SetFormatter(&logrus.TextFormatter{DisableQuote: true})
		} else {
			// Use a JSONFormatter for non-local environments because Google Cloud Logging works well with JSON-formatted log entries
			l.SetFormatter(&logrus.JSONFormatter{})
		}

	})
}

func isDevEnv() bool {
	return env.GetString(context.Background(), "ENV") != "production"
}
