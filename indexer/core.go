package indexer

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"time"

	"cloud.google.com/go/storage"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/indexer/refresh"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
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

	setDefaults("indexer")
	initSentry()
	initLogger()

	var s *storage.Client
	if viper.GetString("ENV") == "local" {
		s = media.NewLocalStorageClient(context.Background(), "./_deploy/service-key-dev.json")
	} else {
		s = media.NewStorageClient(context.Background())
	}
	tokenRepo, contractRepo, addressFilterRepo := newRepos(s)
	ethClient := rpc.NewEthSocketClient()
	ipfsClient := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()

	// overrides for where the indexer starts and stops
	startingBlock, maxBlock := getBlockRangeFromArgs()

	if viper.GetString("ENV") == "production" {
		rpcEnabled = true
	}

	i := newIndexer(ethClient, ipfsClient, arweaveClient, s, tokenRepo, contractRepo, addressFilterRepo, persist.Chain(viper.GetInt("CHAIN")), defaultTransferEvents, nil, startingBlock, maxBlock)

	router := gin.Default()

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logger.For(nil).Info("Registering handlers...")
	return handlersInit(router, i, tokenRepo, contractRepo, ethClient, ipfsClient, arweaveClient, s), i
}

func getBlockRangeFromArgs() (*uint64, *uint64) {
	var startingBlock, maxBlock *uint64
	if len(os.Args) > 1 {
		start, err := strconv.ParseUint(os.Args[1], 10, 64)
		if err != nil {
			panic(err)
		}
		startingBlock = &start
	}
	if len(os.Args) > 2 {
		max, err := strconv.ParseUint(os.Args[2], 10, 64)
		if err != nil {
			panic(err)
		}
		maxBlock = &max
	}
	return startingBlock, maxBlock
}

func coreInitServer() *gin.Engine {
	ctx := sentry.SetHubOnContext(context.Background(), sentry.CurrentHub())

	localKeyPath := "./_deploy/service-key-dev.json"
	if len(os.Args) > 1 {
		if os.Args[1] == "prod" {
			localKeyPath = "./_deploy/service-key.json"
		}
	}
	setDefaults("indexer-server")
	initSentry()
	initLogger()

	var s *storage.Client
	if viper.GetString("ENV") == "local" {
		s = media.NewLocalStorageClient(context.Background(), localKeyPath)
	} else {
		s = media.NewStorageClient(context.Background())
	}
	tokenRepo, contractRepo, addressFilterRepo := newRepos(s)
	ethClient := rpc.NewEthSocketClient()
	ipfsClient := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()

	if viper.GetString("ENV") == "production" {
		rpcEnabled = true
	}

	router := gin.Default()

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logger.For(ctx).Info("Registering handlers...")

	queueChan := make(chan processTokensInput)
	t := newThrottler()

	i := newIndexer(ethClient, ipfsClient, arweaveClient, s, tokenRepo, contractRepo, addressFilterRepo, persist.Chain(viper.GetInt("CHAIN")), defaultTransferEvents, nil, nil, nil)

	go processMedialessTokens(configureRootContext(), queueChan, tokenRepo, contractRepo, ipfsClient, ethClient, arweaveClient, s, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), t)
	return handlersInitServer(router, queueChan, tokenRepo, contractRepo, ethClient, ipfsClient, arweaveClient, s, i)
}

func setDefaults(service string) {
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
	viper.SetDefault("VERSION", "")

	viper.AutomaticEnv()

	if viper.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
	} else {
		envFile := util.ResolveEnvFile(service)
		util.LoadEnvFile(envFile)
	}

	util.EnvVarMustExist("RPC_URL", "")
	if viper.GetString("ENV") != "local" {
		util.EnvVarMustExist("SENTRY_DSN", "")
		util.EnvVarMustExist("VERSION", "")
	}
}

func newRepos(storageClient *storage.Client) (persist.TokenRepository, persist.ContractRepository, refresh.AddressFilterRepository) {
	pgClient := postgres.NewClient()
	return postgres.NewTokenRepository(pgClient), postgres.NewContractRepository(pgClient), refresh.AddressFilterRepository{Bucket: storageClient.Bucket(viper.GetString("GCLOUD_TOKEN_LOGS_BUCKET"))}
}

func newThrottler() *throttle.Locker {
	return throttle.NewThrottleLocker(redis.NewCache(redis.IndexerServerThrottleDB), time.Minute*5)
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
	if viper.GetString("ENV") != "production" {
		logger.For(ctx).Logger.SetLevel(logrus.DebugLevel)
	}
	logger.For(ctx).Logger.SetReportCaller(true)
	logger.For(ctx).Logger.AddHook(sentryutil.SentryLoggerHook)
	return sentry.SetHubOnContext(ctx, sentry.CurrentHub())
}
