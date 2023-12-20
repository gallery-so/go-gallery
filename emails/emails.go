package emails

import (
	"context"
	"net/http"
	"os"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
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

	loaders := dataloader.NewLoaders(context.Background(), queries, false, tracing.DataloaderPreFetchHook, tracing.DataloaderPostFetchHook)

	sendgridClient := sendgrid.NewSendClient(env.GetString("SENDGRID_API_KEY"))

	http.DefaultClient = &http.Client{Transport: tracing.NewTracingTransport(http.DefaultTransport, false)}
	stg := rpc.NewStorageClient(context.Background())

	router := gin.Default()

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())

	if env.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logger.For(nil).Info("Registering handlers...")

	r := redis.NewCache(redis.EmailThrottleCache)
	p := publicapi.New(context.Background(), false, postgres.NewRepositories(postgres.MustCreateClient(), pgxClient), queries, http.DefaultClient, nil, nil, nil, stg, nil, nil, nil, nil, nil, redis.NewCache(redis.FeedCache), nil, nil, nil)

	return handlersInitServer(router, loaders, queries, sendgridClient, r, stg, p)
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
	viper.SetDefault("SENDGRID_DIGEST_TEMPLATE_ID", "d-0b9b6b0b0b5e4b6e9b0b0b5e4b6e9b0b")
	viper.SetDefault("SENDGRID_UNSUBSCRIBE_NOTIFICATIONS_GROUP_ID", 20676)
	viper.SetDefault("SENDGRID_UNSUBSCRIBE_DIGEST_GROUP_ID", 46079)
	viper.SetDefault("SCHEDULER_AUDIENCE", "")
	viper.SetDefault("GOOGLE_CLOUD_PROJECT", "gallery-dev-322005")
	viper.SetDefault("ADMIN_PASS", "admin")
	viper.SetDefault("EMAILS_TASK_SECRET", "emails-task-secret")
	viper.SetDefault("RETOOL_AUTH_TOKEN", "")
	viper.SetDefault("CONFIGURATION_BUCKET", "gallery-dev-configurations")

	viper.AutomaticEnv()

	if env.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
	} else {
		fi := "local"
		if len(os.Args) > 1 {
			fi = os.Args[1]
		}
		envFile := util.ResolveEnvFile("emails", fi)
		util.LoadEncryptedEnvFile(envFile)
	}

	if env.GetString("ENV") != "local" {
		util.VarNotSetTo("SENTRY_DSN", "")
		util.VarNotSetTo("VERSION", "")
		util.VarNotSetTo("SENDGRID_API_KEY", "")
		util.VarNotSetTo("EMAIL_VERIFICATION_JWT_SECRET", "")
		util.VarNotSetTo("FROM_EMAIL", "")
	}
}

func initSentry() {
	if env.GetString("ENV") == "local" {
		logger.For(nil).Info("skipping sentry init")
		return
	}

	logger.For(nil).Info("initializing sentry...")

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              env.GetString("SENTRY_DSN"),
		Environment:      env.GetString("ENV"),
		TracesSampleRate: env.GetFloat64("SENTRY_TRACES_SAMPLE_RATE"),
		Release:          env.GetString("VERSION"),
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

func isDevEnv() bool {
	return env.GetString("ENV") != "production"
}
