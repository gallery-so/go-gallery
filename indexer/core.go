package indexer

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/storage"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	sqlc "github.com/mikeydub/go-gallery/db/sqlc/indexergen"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/api/option"
)

// Init initializes the indexer
func Init() {
	router, i := coreInit()
	logger.For(nil).Info("Starting indexer...")
	go i.Start(configureRootContext())
	http.Handle("/", router)
}

// InitServer initializes the indexer server
func InitServer() {
	router := coreInitServer()
	logger.For(nil).Info("Starting indexer server...")
	http.Handle("/", router)
}

func coreInit() (*gin.Engine, *indexer) {

	setDefaults("_local/app-local-indexer.yaml")
	initSentry()
	initLogger()

	tokenRepo, contractRepo, blockFilterRepo := newRepos()
	var s *storage.Client
	var err error
	if viper.GetString("ENV") != "local" {
		s, err = storage.NewClient(context.Background())
	} else {
		s, err = storage.NewClient(context.Background(), option.WithCredentialsFile("./_deploy/service-key-dev.json"))
	}
	if err != nil {
		panic(err)
	}
	ethClient := rpc.NewEthClient()
	ipfsClient := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()

	i := newIndexer(ethClient, ipfsClient, arweaveClient, s, tokenRepo, contractRepo, blockFilterRepo, persist.Chain(viper.GetInt("CHAIN")), defaultTransferEvents)

	router := gin.Default()

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logger.For(nil).Info("Registering handlers...")
	return handlersInit(router, i, tokenRepo, contractRepo, ethClient, ipfsClient, arweaveClient, s), i
}

func coreInitServer() *gin.Engine {
	ctx := sentry.SetHubOnContext(context.Background(), sentry.CurrentHub())

	path := "_local/app-local-indexer-server.yaml"
	if len(os.Args) > 0 {
		if os.Args[0] == "prod" {
			path = "_local/app-prod-indexer-server.yaml"
		}
	}
	setDefaults(path)
	initSentry()
	initLogger()

	tokenRepo, contractRepo, blockFilterRepo := newRepos()
	queries := sqlc.New(postgres.NewPgxClient())

	var s *storage.Client
	var err error
	if viper.GetString("ENV") != "local" {
		s, err = storage.NewClient(ctx)
	} else {
		s, err = storage.NewClient(ctx, option.WithCredentialsFile("./_deploy/service-key-dev.json"))
	}
	if err != nil {
		panic(err)
	}
	ethClient := rpc.NewEthClient()
	ipfsClient := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()

	router := gin.Default()

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logger.For(ctx).Info("Registering handlers...")

	queueChan := make(chan processTokensInput)
	refreshQueue := NewRefreshQueue()
	refreshLock := NewRefreshLock()
	t := newThrottler()

	go processDeepRefreshes(sentryutil.NewSentryHubContext(ctx), refreshQueue, refreshLock, tokenRepo, ethClient, ipfsClient, arweaveClient, s, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), contractRepo, persist.Chain(viper.GetInt("CHAIN")), blockFilterRepo, queries)
	go processMedialessTokens(configureRootContext(), queueChan, tokenRepo, contractRepo, ipfsClient, ethClient, arweaveClient, s, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), t)
	return handlersInitServer(router, queueChan, tokenRepo, contractRepo, ethClient, ipfsClient, arweaveClient, s, refreshQueue)
}

func setDefaults(envFilePath string) {
	viper.SetDefault("RPC_URL", "")
	viper.SetDefault("IPFS_URL", "https://gallery.infura-ipfs.io")
	viper.SetDefault("IPFS_API_URL", "https://ipfs.infura.io:5001")
	viper.SetDefault("IPFS_PROJECT_ID", "")
	viper.SetDefault("IPFS_PROJECT_SECRET", "")
	viper.SetDefault("CHAIN", 0)
	viper.SetDefault("ENV", "local")
	viper.SetDefault("GCLOUD_TOKEN_LOGS_BUCKET", "dev-eth-token-logs")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "dev-token-content")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5433)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("IMGIX_API_KEY", "")
	viper.SetDefault("GAE_VERSION", "")

	viper.AutomaticEnv()

	if viper.GetString("ENV") == "local" {
		path, err := util.FindFile(envFilePath, 3)
		if err != nil {
			panic(err)
		}

		viper.SetConfigFile(path)
		if err := viper.ReadInConfig(); err != nil {
			panic(fmt.Sprintf("error reading viper config: %s\nmake sure your _local directory is decrypted and up-to-date", err))
		}
	}

	util.EnvVarMustExist("RPC_URL", "")
	if viper.GetString("ENV") != "local" {
		util.EnvVarMustExist("SENTRY_DSN", "")
		util.EnvVarMustExist("GAE_VERSION", "")
	}
}

func newRepos() (persist.TokenRepository, persist.ContractRepository, postgres.BlockFilterRepository) {
	pgClient := postgres.NewClient()
	pgx := postgres.NewPgxClient()
	return postgres.NewTokenRepository(pgClient), postgres.NewContractRepository(pgClient), postgres.BlockFilterRepository{Queries: sqlc.New(pgx)}
}

func newThrottler() *throttle.Locker {
	return throttle.NewThrottleLocker(redis.NewCache(redis.IndexerServerThrottleDB), time.Minute*5)
}

func initSentry() {
	// XXX: if viper.GetString("ENV") == "local" {
	// XXX: 	logger.For(nil).Info("skipping sentry init")
	// XXX: 	return
	// XXX: }

	logger.For(nil).Info("initializing sentry...")

	err := sentry.Init(sentry.ClientOptions{
		Dsn:         viper.GetString("SENTRY_DSN"),
		Environment: viper.GetString("ENV"),
		// XXX: TracesSampleRate: viper.GetFloat64("SENTRY_TRACES_SAMPLE_RATE"),
		TracesSampleRate: 1.0,
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
	if viper.GetString("ENV") != "production" {
		logger.For(ctx).Logger.SetLevel(logrus.DebugLevel)
	}
	logger.For(ctx).Logger.SetReportCaller(true)
	logger.For(ctx).Logger.AddHook(sentryutil.SentryLoggerHook)
	return sentry.SetHubOnContext(ctx, sentry.CurrentHub())
}
