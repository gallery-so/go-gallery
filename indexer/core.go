package indexer

import (
	"context"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/db/gen/indexerdb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/service/rpc/arweave"
	"github.com/mikeydub/go-gallery/service/rpc/ipfs"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// Init initializes the indexer
func Init(fromBlock, toBlock *uint64, quietLogs, enableRPC bool) {
	router, i := coreInit(fromBlock, toBlock, quietLogs, enableRPC)
	logger.For(nil).Info("Starting indexer...")
	go i.Start(sentry.SetHubOnContext(context.Background(), sentry.CurrentHub()))
	http.Handle("/", router)
}

// InitServer initializes the indexer server
func InitServer(quietLogs, enableRPC bool) {
	router := coreInitServer(quietLogs, enableRPC)
	logger.For(nil).Info("Starting indexer server...")
	http.Handle("/", router)
}

func coreInit(fromBlock, toBlock *uint64, quietLogs, enableRPC bool) (*gin.Engine, *indexer) {
	InitSentry()
	logger.InitWithGCPDefaults()
	logger.SetLoggerOptions(func(logger *logrus.Logger) {
		logger.AddHook(sentryutil.SentryLoggerHook)
		logger.SetLevel(logrus.InfoLevel)
		if env.GetString("ENV") != "production" && !quietLogs {
			logger.SetLevel(logrus.DebugLevel)
		}
	})

	s := rpc.NewStorageClient(context.Background())
	contractRepo := postgres.NewContractRepository(postgres.MustCreateClient())
	iQueries := indexerdb.New(postgres.NewPgxClient())
	bQueries := coredb.New(postgres.NewPgxClient(postgres.WithHost(env.GetString("BACKEND_POSTGRES_HOST")), postgres.WithUser(env.GetString("BACKEND_POSTGRES_USER")), postgres.WithPassword(env.GetString("BACKEND_POSTGRES_PASSWORD")), postgres.WithPort(env.GetInt("BACKEND_POSTGRES_PORT"))))
	tClient := task.NewClient(context.Background())
	ethClient := rpc.NewEthSocketClient()
	ipfsClient := ipfs.NewShell()
	arweaveClient := arweave.NewClient()

	if env.GetString("ENV") == "production" || enableRPC {
		rpcEnabled = true
	}

	i := newIndexer(ethClient, &http.Client{Timeout: 10 * time.Minute}, ipfsClient, arweaveClient, s, iQueries, bQueries, tClient, contractRepo, persist.Chain(env.GetInt("CHAIN")), defaultTransferEvents, nil, fromBlock, toBlock)

	router := gin.Default()

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())

	if env.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
	}

	logger.For(nil).Info("Registering handlers...")
	return handlersInit(router, i, contractRepo, ethClient, ipfsClient, arweaveClient, s), i
}

func coreInitServer(quietLogs, enableRPC bool) *gin.Engine {
	ctx := sentry.SetHubOnContext(context.Background(), sentry.CurrentHub())
	InitSentry()
	logger.InitWithGCPDefaults()
	logger.SetLoggerOptions(func(logger *logrus.Logger) {
		logger.SetLevel(logrus.InfoLevel)
		if env.GetString("ENV") != "production" && !quietLogs {
			logger.SetLevel(logrus.DebugLevel)
		}
	})

	s := rpc.NewStorageClient(context.Background())
	contractRepo := postgres.NewContractRepository(postgres.MustCreateClient())
	iQueries := indexerdb.New(postgres.NewPgxClient())
	bQueries := coredb.New(postgres.NewPgxClient(postgres.WithHost(env.GetString("BACKEND_POSTGRES_HOST")), postgres.WithUser(env.GetString("BACKEND_POSTGRES_USER")), postgres.WithPassword(env.GetString("BACKEND_POSTGRES_PASSWORD")), postgres.WithPort(env.GetInt("BACKEND_POSTGRES_PORT"))))
	tClient := task.NewClient(context.Background())
	ethClient := rpc.NewEthSocketClient()
	ipfsClient := ipfs.NewShell()
	arweaveClient := arweave.NewClient()

	if env.GetString("ENV") == "production" || enableRPC {
		rpcEnabled = true
	}

	router := gin.Default()

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())

	if env.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logger.For(ctx).Info("Registering handlers...")

	httpClient := &http.Client{Timeout: 10 * time.Minute, Transport: tracing.NewTracingTransport(http.DefaultTransport, false)}

	i := newIndexer(ethClient, httpClient, ipfsClient, arweaveClient, s, iQueries, bQueries, tClient, contractRepo, persist.Chain(env.GetInt("CHAIN")), defaultTransferEvents, nil, nil, nil)
	return handlersInitServer(router, contractRepo, ethClient, httpClient, ipfsClient, arweaveClient, s, i)
}

func SetDefaults() {
	viper.SetDefault("RPC_URL", "")
	viper.SetDefault("IPFS_URL", "https://gallery.infura-ipfs.io")
	viper.SetDefault("IPFS_API_URL", "https://ipfs.infura.io:5001")
	viper.SetDefault("FALLBACK_IPFS_URL", "https://ipfs.io")
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
	viper.SetDefault("BACKEND_POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("BACKEND_POSTGRES_PORT", 5432)
	viper.SetDefault("BACKEND_POSTGRES_USER", "postgres")
	viper.SetDefault("BACKEND_POSTGRES_PASSWORD", "")
	viper.SetDefault("BACKEND_POSTGRES_DB", "postgres")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("IMGIX_API_KEY", "")
	viper.SetDefault("VERSION", "")
	viper.SetDefault("ALCHEMY_API_URL", "")
	viper.SetDefault("ALCHEMY_NFT_API_URL", "")
	viper.SetDefault("TASK_QUEUE_HOST", "localhost:8123")
	viper.SetDefault("TOKEN_PROCESSING_QUEUE", "projects/gallery-local/locations/here/queues/token-processing")
	viper.SetDefault("TOKEN_PROCESSING_URL", "http://localhost:6500")
	viper.AutomaticEnv()
}

func LoadConfigFile(service string, manualEnv string) {
	if env.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
		return
	}
	util.LoadEncryptedEnvFile(util.ResolveEnvFile(service, manualEnv))
}

func ValidateEnv() {
	util.VarNotSetTo("RPC_URL", "")
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
		Dsn:         env.GetString("SENTRY_DSN"),
		Environment: env.GetString("ENV"),
		TracesSampler: sentry.TracesSamplerFunc(func(ctx sentry.SamplingContext) sentry.Sampled {
			if ctx.Span.Op == rpc.GethSocketOpName {
				return sentry.UniformTracesSampler(0.01).Sample(ctx)
			}
			return sentry.UniformTracesSampler(env.GetFloat64("SENTRY_TRACES_SAMPLE_RATE")).Sample(ctx)
		}),
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
