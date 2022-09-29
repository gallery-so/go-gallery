package tokenprocessing

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// InitServer initializes the indexer server
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
	s := media.NewStorageClient(ctx, "./_deploy/service-key-dev.json")

	http.DefaultClient = &http.Client{Transport: tracing.NewTracingTransport(http.DefaultTransport, false, true)}
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
	// mc := server.NewMultichainProvider(repos, redis.NewCache(redis.CommunitiesDB), rpc.NewEthClient(), http.DefaultClient, ipfsClient, arweaveClient, storageClient, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"))
	// mediaQueue := make(chan ProcessMediaInput)
	// collectionTokensQueue := make(chan ProcessCollectionTokensRefreshInput)
	// startJobs(mediaQueue, repos, ipfsClient, arweaveClient, s, t, collectionTokensQueue, mc)
	return handlersInitServer(router, repos, ipfsClient, arweaveClient, s, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), t)
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
	viper.SetDefault("SELF_HOST", "http://localhost:6500")

	viper.AutomaticEnv()

	if viper.GetString("ENV") == "local" {

		filePath := "_local/app-local-mediaprocessing.yaml"
		if len(os.Args) > 1 {
			if os.Args[1] == "dev" {
				filePath = "_local/app-dev-mediaprocessing.yaml"
			} else if os.Args[1] == "prod" {
				filePath = "_local/app-prod-mediaprocessing.yaml"
			}
		}

		// Tests can run from directories deeper in the source tree, so we need to search parent directories to find this config file
		path, err := util.FindFile(filePath, 3)
		if err != nil {
			panic(err)
		}

		viper.SetConfigFile(path)
		if err := viper.ReadInConfig(); err != nil {
			panic(fmt.Sprintf("error reading viper config: %s\nmake sure your _local directory is decrypted and up-to-date", err))
		}
	}

	if viper.GetString("ENV") != "local" && viper.GetString("SENTRY_DSN") == "" {
		panic("SENTRY_DSN must be set")
	}

}

func newThrottler() *throttle.Locker {
	return throttle.NewThrottleLocker(redis.NewCache(redis.TokenProcessingThrottleDB), time.Minute*5)
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
