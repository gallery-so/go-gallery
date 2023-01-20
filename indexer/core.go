package indexer

import (
	"context"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/indexer/refresh"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/throttle"
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
	initSentry()
	logger.InitWithGCPDefaults()
	logger.SetLoggerOptions(func(logger *logrus.Logger) {
		logger.AddHook(sentryutil.SentryLoggerHook)
		logger.SetLevel(logrus.InfoLevel)
		if viper.GetString("ENV") != "production" && !quietLogs {
			logger.SetLevel(logrus.DebugLevel)
		}
	})

	s := media.NewStorageClient(context.Background())
	tokenRepo, contractRepo, addressFilterRepo := newRepos(s)
	ethClient := rpc.NewEthSocketClient()
	ipfsClient := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()

	if viper.GetString("ENV") == "production" || enableRPC {
		rpcEnabled = true
	}

	i := newIndexer(ethClient, ipfsClient, arweaveClient, s, tokenRepo, contractRepo, addressFilterRepo, persist.Chain(viper.GetInt("CHAIN")), defaultTransferEvents, nil, fromBlock, toBlock)

	router := gin.Default()

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
	}

	logger.For(nil).Info("Registering handlers...")
	return handlersInit(router, i, tokenRepo, contractRepo, ethClient, ipfsClient, arweaveClient, s), i
}

func coreInitServer(quietLogs, enableRPC bool) *gin.Engine {
	ctx := sentry.SetHubOnContext(context.Background(), sentry.CurrentHub())
	initSentry()
	logger.InitWithGCPDefaults()
	logger.SetLoggerOptions(func(logger *logrus.Logger) {
		logger.SetLevel(logrus.InfoLevel)
		if viper.GetString("ENV") != "production" && !quietLogs {
			logger.SetLevel(logrus.DebugLevel)
		}
	})

	s := media.NewStorageClient(context.Background())
	tokenRepo, contractRepo, addressFilterRepo := newRepos(s)
	ethClient := rpc.NewEthSocketClient()
	ipfsClient := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()

	if viper.GetString("ENV") == "production" || enableRPC {
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

	go processIncompleteTokens(ctx, queueChan, tokenRepo, contractRepo, ipfsClient, ethClient, arweaveClient, s, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), t)
	return handlersInitServer(router, queueChan, tokenRepo, contractRepo, ethClient, ipfsClient, arweaveClient, s, i)
}

func SetDefaults() {
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
}

func LoadConfigFile(service string, manualEnv string) {
	if viper.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
		return
	}
	util.LoadEncryptedEnvFile(util.ResolveEnvFile(service, manualEnv))
}

func ValidateEnv() {
	util.VarNotSetTo("RPC_URL", "")
	if viper.GetString("ENV") != "local" {
		util.VarNotSetTo("SENTRY_DSN", "")
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
		Dsn:         viper.GetString("SENTRY_DSN"),
		Environment: viper.GetString("ENV"),
		TracesSampler: sentry.TracesSamplerFunc(func(ctx sentry.SamplingContext) sentry.Sampled {
			if ctx.Span.Op == rpc.GethSocketOpName {
				return sentry.UniformTracesSampler(0.01).Sample(ctx)
			}
			return sentry.UniformTracesSampler(viper.GetFloat64("SENTRY_TRACES_SAMPLE_RATE")).Sample(ctx)
		}),
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
