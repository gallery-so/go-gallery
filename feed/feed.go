package feed

import (
	"context"
	"net/http"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4/pgxpool"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

func Init() {
	setDefaults()

	logger.InitWithGCPDefaults()
	initSentry()

	router := coreInit(postgres.NewPgxClient())
	http.Handle("/", router)
}

func coreInit(pgx *pgxpool.Pool) *gin.Engine {
	logger.For(nil).Info("initializing server...")

	router := gin.Default()
	router.Use(middleware.ErrLogger(), middleware.Sentry(true), middleware.Tracing())

	if env.Get[string](context.Background(), "ENV") != "production" {
		gin.SetMode(gin.DebugMode)
	}

	return handlersInit(router, db.New(pgx), task.NewClient(context.Background()))
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("FEED_SECRET", "feed-secret")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("TASK_QUEUE_HOST", "localhost:8123")
	viper.SetDefault("GCLOUD_FEEDBOT_TASK_QUEUE", "projects/gallery-local/locations/here/queues/feedbot")
	viper.SetDefault("FEEDBOT_SECRET", "feed-bot-secret")
	viper.SetDefault("FEED_WINDOW_SIZE", 20)
	viper.SetDefault("GAE_VERSION", "")
	viper.AutomaticEnv()

	if env.Get[string](context.Background(), "ENV") != "local" {
		util.VarNotSetTo("SENTRY_DSN", "")
		util.VarNotSetTo("GAE_VERSION", "")
	}
}

func initSentry() {
	if env.Get[string](context.Background(), "ENV") == "local" {
		logger.For(nil).Info("skipping sentry init")
		return
	}

	logger.For(nil).Info("initializing sentry...")

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              env.Get[string](context.Background(), "SENTRY_DSN"),
		Environment:      env.Get[string](context.Background(), "ENV"),
		TracesSampleRate: env.Get[float64](context.Background(), "SENTRY_TRACES_SAMPLE_RATE"),
		AttachStacktrace: true,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			event = auth.ScrubEventCookies(event, hint)
			event = sentryutil.UpdateErrorFingerprints(event, hint)
			return event
		},
	})

	if err != nil {
		logger.For(nil).Fatalf("failed to start sentry: %s", err)
	}
}
