package activitystats

import (
	"context"
	"github.com/mikeydub/go-gallery/service/farcaster"
	"net/http"
	"os"

	"cloud.google.com/go/pubsub"
	"github.com/bsm/redislock"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/notifications"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/pubsub/gcp"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
)

// InitServer initializes the autosocial server
func InitServer() {
	setDefaults()
	ctx := context.Background()
	router := CoreInitServer(ctx)

	logger.For(nil).Info("Starting activity stats server...")
	http.Handle("/", router)
}

func CoreInitServer(ctx context.Context) *gin.Engine {
	InitSentry()
	logger.InitWithGCPDefaults()

	http.DefaultClient = &http.Client{Transport: tracing.NewTracingTransport(http.DefaultTransport, false)}

	router := gin.Default()
	pgx := postgres.NewPgxClient()
	queries := coredb.New(pgx)
	stg := rpc.NewStorageClient(ctx)
	pub := gcp.NewClient(ctx)
	lock := redis.NewLockClient(redis.NewCache(redis.NotificationLockCache))
	tc := task.NewClient(ctx)
	neynar := farcaster.NewNeynarAPI(http.DefaultClient, nil, queries)

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger(), useEventHandler(queries, pub, tc, lock, neynar))
	router.POST("/calculate_activity_badges", middleware.CloudSchedulerMiddleware, autoCalculateTopActivityBadges(queries, stg, pgx))
	router.POST("/recalculate_activity_badges", middleware.RetoolAuthRequired, recalculateTopActivityBadges(queries, stg, pgx))
	router.POST("/update_top_conf", middleware.RetoolAuthRequired, updateTopActivityConfiguration(stg))
	router.GET("/get_top_conf", middleware.RetoolAuthRequired, getTopActivityConfiguration(stg))

	return router
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("GAE_VERSION", "")
	viper.SetDefault("GOOGLE_CLOUD_PROJECT", "gallery-dev-322005")
	viper.SetDefault("CONFIGURATION_BUCKET", "gallery-dev-configurations")
	viper.SetDefault("SCHEDULER_AUDIENCE", "")
	viper.SetDefault("BASIC_AUTH_TOKEN_RETOOL", "")
	viper.SetDefault("BASIC_AUTH_TOKEN_MONITORING", "")
	viper.SetDefault("TASK_QUEUE_HOST", "localhost:8123")
	viper.SetDefault("PUBSUB_EMULATOR_HOST", "[::1]:8085")
	viper.SetDefault("PUBSUB_TOPIC_NEW_NOTIFICATIONS", "dev-new-notifications")
	viper.SetDefault("GCLOUD_PUSH_NOTIFICATIONS_QUEUE", "projects/gallery-local/locations/here/queues/push-notifications")
	viper.SetDefault("PUSH_NOTIFICATIONS_SECRET", "push-notifications-secret")
	viper.SetDefault("PUSH_NOTIFICATIONS_URL", "http://localhost:8000")

	viper.AutomaticEnv()

	if env.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
	} else {
		fi := "local"
		if len(os.Args) > 1 {
			fi = os.Args[1]
		}
		envFile := util.ResolveEnvFile("activitystats", fi)
		util.LoadEncryptedEnvFile(envFile)
	}

	if env.GetString("ENV") != "local" {
		util.VarNotSetTo("SENTRY_DSN", "")
	}
}

func InitSentry() {
	if env.GetString("ENV") == "local" {
		logger.For(nil).Info("skipping sentry init")
		return
	}

	logger.For(nil).Info("initializing sentry...")

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              env.GetString("SENTRY_DSN"),
		Environment:      env.GetString("ENV"),
		TracesSampleRate: env.GetFloat64("SENTRY_TRACES_SAMPLE_RATE"),
		Release:          env.GetString("GAE_VERSION"),
		AttachStacktrace: true,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			event = auth.ScrubEventCookies(event, hint)
			return event
		},
	})

	if err != nil {
		logger.For(nil).Fatalf("failed to start sentry: %s", err)
	}
}

func useEventHandler(q *coredb.Queries, p *pubsub.Client, t *task.Client, l *redislock.Client, n *farcaster.NeynarAPI) gin.HandlerFunc {
	return func(c *gin.Context) {
		event.AddTo(c, false, notifications.New(q, p, t, l, false), q, t, n)
		c.Next()
	}
}
