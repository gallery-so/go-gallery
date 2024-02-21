package emails

import (
	"context"
	"net/http"
	"os"

	"github.com/Khan/genqlient/graphql"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/pubsub/gcp"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/store"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
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

	router := gin.Default()

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())

	if env.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logger.For(nil).Info("Registering handlers...")

	r := redis.NewCache(redis.EmailThrottleCache)
	lock := redis.NewLockClient(redis.NewCache(redis.NotificationLockCache))
	psub := gcp.NewClient(context.Background())
	t := task.NewClient(context.Background())
	b := store.NewBucketStorer(rpc.NewStorageClient(context.Background()), env.GetString("CONFIGURATION_BUCKET"))
	gql := graphql.NewClient(env.GetString("GALLERY_API"), http.DefaultClient)

	return handlersInitServer(router, loaders, queries, sendgridClient, r, &b, psub, t, lock, &gql)
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
	viper.SetDefault("BASIC_AUTH_TOKEN_RETOOL", "")
	viper.SetDefault("BASIC_AUTH_TOKEN_MONITORING", "")
	viper.SetDefault("CONFIGURATION_BUCKET", "gallery-dev-configurations")
	viper.SetDefault("TASK_QUEUE_HOST", "localhost:8123")
	viper.SetDefault("PUBSUB_EMULATOR_HOST", "[::1]:8085")
	viper.SetDefault("PUBSUB_TOPIC_NEW_NOTIFICATIONS", "dev-new-notifications")
	viper.SetDefault("GCLOUD_PUSH_NOTIFICATIONS_QUEUE", "projects/gallery-local/locations/here/queues/push-notifications")
	viper.SetDefault("PUSH_NOTIFICATIONS_SECRET", "push-notifications-secret")
	viper.SetDefault("PUSH_NOTIFICATIONS_URL", "http://localhost:8000")
	viper.SetDefault("GALLERY_API", "http://localhost:4000/glry/graphql/query")

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
