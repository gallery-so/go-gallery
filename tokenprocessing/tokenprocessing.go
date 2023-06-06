package tokenprocessing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/metric"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/redis"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const sentryTokenContextName = "NFT context" // Sentry excludes contexts that contain "token" so we use "NFT" instead

// InitServer initializes the mediaprocessing server
func InitServer() {
	setDefaults()
	ctx := context.Background()
	c := server.ClientInit(ctx)
	provider, _ := server.NewMultichainProvider(ctx)
	router := CoreInitServer(c, provider)
	logger.For(nil).Info("Starting tokenprocessing server...")
	http.Handle("/", router)
}

func CoreInitServer(c *server.Clients, mc *multichain.Provider) *gin.Engine {
	InitSentry()
	logger.InitWithGCPDefaults()

	http.DefaultClient = &http.Client{Transport: tracing.NewTracingTransport(http.DefaultTransport, false)}

	router := gin.Default()

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())

	if env.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logger.For(nil).Info("Registering handlers...")

	t := newThrottler()

	tp := NewTokenProcessor(c.Queries, c.EthClient, mc, c.IPFSClient, c.ArweaveClient, c.StorageClient, env.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), c.Repos.TokenRepository, metric.NewLogMetricReporter())

	return handlersInitServer(router, tp, mc, c.Repos, t)
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
	viper.SetDefault("ALCHEMY_NFT_API_URL", "")
	viper.SetDefault("POAP_API_KEY", "")
	viper.SetDefault("POAP_AUTH_TOKEN", "")

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
		setTokenTags(scope, job.token.Chain, job.contract.Address, job.token.TokenID, job.token.ID)
		setTokenContext(scope, job.token.Chain, job.contract.Address, job.token.TokenID, isSpamToken(job))
	})
}

func setTokenTags(scope *sentry.Scope, chain persist.Chain, contractAddress persist.Address, tokenID persist.TokenID, id persist.DBID) {
	scope.SetTag("chain", fmt.Sprintf("%d", chain))
	scope.SetTag("contractAddress", contractAddress.String())
	scope.SetTag("nftID", string(tokenID))
	scope.SetTag("tokenDBID", string(id))
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
	scope.SetTag("log", "go/tp-runs/"+runID.String())
}

// isSpamToken returns true if the token is marked as spam.
func isSpamToken(job tokenProcessingJob) bool {
	isSpam := job.contract.IsProviderMarkedSpam
	isSpam = isSpam || util.GetOptionalValue(job.token.IsProviderMarkedSpam, false)
	isSpam = isSpam || util.GetOptionalValue(job.token.IsUserMarkedSpam, false)
	return isSpam
}

// isBadTokenErr returns true if the error is a bad token error.
func isBadTokenErr(err error) bool {
	var badTokenErr ErrBadToken
	if errors.As(err, &badTokenErr) {
		return true
	}
	return false
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
