package tokenprocessing

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/util"
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

	repos := newRepos(postgres.NewClient())
	var s *storage.Client
	if viper.GetString("ENV") == "local" {
		s = media.NewLocalStorageClient(context.Background(), "./_deploy/service-key-dev.json")
	} else {
		s = media.NewStorageClient(context.Background())
	}

	// XXX: http.DefaultClient = &http.Client{Transport: tracing.NewTracingTransport(http.DefaultTransport, false)}
	ipfsClient := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()

	router := gin.Default()

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logger.For(ctx).Info("Registering handlers...")

	t := newThrottler()
	queries := coredb.New(postgres.NewPgxClient())
	mc := server.NewMultichainProvider(repos, queries, redis.NewCache(redis.CommunitiesDB), rpc.NewEthClient(), http.DefaultClient, ipfsClient, arweaveClient, s, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), task.NewClient(context.Background()))

	return handlersInitServer(router, mc, repos, ipfsClient, arweaveClient, s, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), t)
}

func setDefaults() {
	viper.SetDefault("IPFS_URL", "https://gallery.infura-ipfs.io")
	viper.SetDefault("IPFS_API_URL", "https://ipfs.infura.io:5001")
	viper.SetDefault("IPFS_PROJECT_ID", "")
	viper.SetDefault("IPFS_PROJECT_SECRET", "")
	viper.SetDefault("CHAIN", 0)
	viper.SetDefault("ENV", "local")
	viper.SetDefault("GCLOUD_TOKEN_LOGS_BUCKET", "dev-eth-token-logs")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "dev-token-content")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("IMGIX_API_KEY", "")
	viper.SetDefault("VERSION", "")

	viper.AutomaticEnv()

	if viper.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
	} else {
		envFile := util.ResolveEnvFile("tokenprocessing")
		util.LoadEnvFile(envFile)
	}

	if viper.GetString("ENV") != "local" {
		util.EnvVarMustExist("SENTRY_DSN", "")
		util.EnvVarMustExist("VERSION", "")
	}
}

func newThrottler() *throttle.Locker {
	return throttle.NewThrottleLocker(redis.NewCache(redis.TokenProcessingThrottleDB), time.Minute*30)
}

func initSentry() {
	// XXX: if viper.GetString("ENV") == "local" {
	// XXX: 	logger.For(nil).Info("skipping sentry init")
	// XXX: 	return
	// XXX: }

	logger.For(nil).Info("initializing sentry...")

	err := sentry.Init(sentry.ClientOptions{
		// XXX: Dsn:         viper.GetString("SENTRY_DSN"),
		Dsn:         "https://99bac248072f4f8187f9e98115e46229@o1135798.ingest.sentry.io/6771759",
		Environment: viper.GetString("ENV"),
		// XXX: TracesSampleRate: viper.GetFloat64("SENTRY_TRACES_SAMPLE_RATE"),
		TracesSampleRate: 1,
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

func newRepos(db *sql.DB) *persist.Repositories {
	galleriesCacheToken := redis.NewCache(1)
	galleryTokenRepo := postgres.NewGalleryRepository(db, galleriesCacheToken)

	return &persist.Repositories{
		UserRepository:        postgres.NewUserRepository(db),
		NonceRepository:       postgres.NewNonceRepository(db),
		LoginRepository:       postgres.NewLoginRepository(db),
		TokenRepository:       postgres.NewTokenGalleryRepository(db, galleryTokenRepo),
		CollectionRepository:  postgres.NewCollectionTokenRepository(db, galleryTokenRepo),
		GalleryRepository:     galleryTokenRepo,
		ContractRepository:    postgres.NewContractGalleryRepository(db),
		BackupRepository:      postgres.NewBackupRepository(db),
		MembershipRepository:  postgres.NewMembershipRepository(db),
		EarlyAccessRepository: postgres.NewEarlyAccessRepository(db),
		WalletRepository:      postgres.NewWalletRepository(db),
		AdmireRepository:      postgres.NewAdmireRepository(db),
		CommentRepository:     postgres.NewCommentRepository(db),
	}
}

// configureRootContext configures the main context from which other contexts are derived.
func configureRootContext() context.Context {
	ctx := logger.NewContextWithLogger(context.Background(), logrus.Fields{}, logrus.New())
	logger.For(ctx).Logger.SetReportCaller(true)
	logger.For(ctx).Logger.AddHook(sentryutil.SentryLoggerHook)
	return sentry.SetHubOnContext(ctx, sentry.CurrentHub())
}
