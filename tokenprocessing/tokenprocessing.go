package tokenprocessing

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/metric"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/notifications"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/redis"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
)

const sentryTokenContextName = "NFT context" // Sentry excludes contexts that contain "token" so we use "NFT" instead

// InitServer initializes the mediaprocessing server
func InitServer() {
	setDefaults()
	ctx := context.Background()
	c := server.ClientInit(ctx)
	mc, _ := server.NewMultichainProvider(ctx, setDefaults)
	router := CoreInitServer(ctx, c, mc)
	logger.For(nil).Info("Starting tokenprocessing server...")
	http.Handle("/", router)
}

func CoreInitServer(ctx context.Context, clients *server.Clients, mc *multichain.Provider) *gin.Engine {
	InitSentry()
	logger.InitWithGCPDefaults()

	http.DefaultClient = &http.Client{Transport: tracing.NewTracingTransport(http.DefaultTransport, false)}

	router := gin.Default()

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())

	notificationsHandler := notifications.New(clients.Queries, clients.PubSubClient, clients.TaskClient, redis.NewLockClient(redis.NewCache(redis.NotificationLockCache)))

	router.Use(func(c *gin.Context) {
		event.AddTo(c, false, notificationsHandler, clients.Queries, clients.TaskClient)
	})

	if env.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logger.For(nil).Info("Registering handlers...")

	t := newThrottler()
	tp := NewTokenProcessor(clients.Queries, clients.HTTPClient, mc, clients.IPFSClient, clients.ArweaveClient, clients.StorageClient, env.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), metric.NewLogMetricReporter())

	return handlersInitServer(ctx, router, tp, mc, clients.Repos, t, clients.TaskClient)
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
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("IMGIX_API_KEY", "")
	viper.SetDefault("VERSION", "")
	viper.SetDefault("ALCHEMY_API_URL", "")
	viper.SetDefault("ALCHEMY_OPTIMISM_API_URL", "")
	viper.SetDefault("ALCHEMY_POLYGON_API_URL", "")
	viper.SetDefault("ALCHEMY_ETH_NFT_API_URL", "")
	viper.SetDefault("ALCHEMY_POLYGON_NFT_API_URL", "")
	viper.SetDefault("POAP_API_KEY", "")
	viper.SetDefault("POAP_AUTH_TOKEN", "")
	viper.SetDefault("TOKEN_PROCESSING_URL", "http://localhost:6500")
	viper.SetDefault("TOKEN_PROCESSING_QUEUE", "projects/gallery-local/locations/here/queues/token-processing")
	viper.SetDefault("TASK_QUEUE_HOST", "")
	viper.SetDefault("GOOGLE_CLOUD_PROJECT", "gallery-dev-322005")
	viper.SetDefault("PUBSUB_EMULATOR_HOST", "")
	viper.SetDefault("PUBSUB_TOPIC_NEW_NOTIFICATIONS", "dev-new-notifications")
	viper.SetDefault("PUBSUB_TOPIC_UPDATED_NOTIFICATIONS", "dev-updated-notifications")
	viper.SetDefault("PUBSUB_SUB_NEW_NOTIFICATIONS", "dev-new-notifications-sub")
	viper.SetDefault("PUBSUB_SUB_UPDATED_NOTIFICATIONS", "dev-updated-notifications-sub")
	viper.SetDefault("RASTERIZER_URL", "http://localhost:3000")
	viper.SetDefault("TEZOS_API_URL", "https://api.tzkt.io")
	viper.SetDefault("ALCHEMY_WEBHOOK_SECRET_ARBITRUM", "")
	viper.SetDefault("ALCHEMY_WEBHOOK_SECRET_ETH", "")

	viper.AutomaticEnv()

	if env.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
	} else {
		fi := "local"
		if len(os.Args) > 1 {
			fi = os.Args[1]
		}
		envFile := util.ResolveEnvFile("tokenprocessing", fi)
		util.LoadEncryptedEnvFile(envFile)
	}

	if env.GetString("ENV") != "local" {
		util.VarNotSetTo("SENTRY_DSN", "")
		util.VarNotSetTo("VERSION", "")
	}
}

func newThrottler() *throttle.Locker {
	return throttle.NewThrottleLocker(redis.NewCache(redis.TokenProcessingThrottleCache), time.Minute*30)
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
		Release:          env.GetString("VERSION"),
		AttachStacktrace: true,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			event = auth.ScrubEventCookies(event, hint)
			event = excludeTokenSpamEvents(event, hint)
			event = excludeBadTokenEvents(event, hint)
			return event
		},
	})

	if err != nil {
		logger.For(nil).Fatalf("failed to start sentry: %s", err)
	}
}

// reportJobError reports an error that occurred while processing a token.
func reportJobError(ctx context.Context, err error, job tokenProcessingJob) {
	sentryutil.ReportError(ctx, err, func(scope *sentry.Scope) {
		setRunTags(scope, job.id)
		setTokenTags(scope, job.token.Chain, job.token.ContractAddress, job.token.TokenID)
		setTokenContext(scope, job.token.Chain, job.token.ContractAddress, job.token.TokenID, job.isSpamJob)
	})
}

func setTokenTags(scope *sentry.Scope, chain persist.Chain, contractAddress persist.Address, tokenID persist.TokenID) {
	scope.SetTag("chain", fmt.Sprintf("%d", chain))
	scope.SetTag("contractAddress", contractAddress.String())
	scope.SetTag("nftID", string(tokenID))
	assetPage := assetURL(chain, contractAddress, tokenID)
	if len(assetPage) > 200 {
		assetPage = "assetURL too long, see token context"
	}
	scope.SetTag("assetURL", assetPage)
}

func assetURL(chain persist.Chain, contractAddress persist.Address, tokenID persist.TokenID) string {
	switch chain {
	case persist.ChainETH:
		return fmt.Sprintf("https://opensea.io/assets/ethereum/%s/%d", contractAddress.String(), tokenID.ToInt())
	case persist.ChainPolygon:
		return fmt.Sprintf("https://opensea.io/assets/matic/%s/%d", contractAddress.String(), tokenID.ToInt())
	case persist.ChainTezos:
		return fmt.Sprintf("https://objkt.com/asset/%s/%d", contractAddress.String(), tokenID.ToInt())
	default:
		return ""
	}
}

func setTokenContext(scope *sentry.Scope, chain persist.Chain, contractAddress persist.Address, tokenID persist.TokenID, isSpam bool) {
	scope.SetContext(sentryTokenContextName, sentry.Context{
		"Chain":           chain,
		"ContractAddress": contractAddress,
		"NftID":           tokenID, // Sentry drops fields containing 'token'
		"IsSpam":          isSpam,
		"AssetURL":        assetURL(chain, contractAddress, tokenID),
	})
}

func setRunTags(scope *sentry.Scope, runID persist.DBID) {
	scope.SetTag("runID", runID.String())
	scope.SetTag("log", "go/tokenruns/"+runID.String())
}

// isBadTokenErr returns true if the error is a bad token error.
func isBadTokenErr(err error) bool {
	return util.ErrorAs[ErrBadToken](err)
}

// excludeTokenSpamEvents excludes events for tokens marked as spam.
func excludeTokenSpamEvents(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	isSpam, ok := event.Contexts[sentryTokenContextName]["IsSpam"].(bool)
	if ok && isSpam {
		return nil
	}
	return event
}

// excludeBadTokenEvents excludes events for bad tokens.
func excludeBadTokenEvents(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	if isBadTokenErr(hint.OriginalException) {
		return nil
	}
	return event
}
