package tokenprocessing

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/api/option"
)

// InitServer initializes the indexer server
func InitServer() {
	router := coreInitServer()
	logger.For(nil).Info("Starting tokenprocessing server...")
	http.Handle("/", router)
}

func coreInitServer() *gin.Engine {
	ctx := sentry.SetHubOnContext(context.Background(), sentry.CurrentHub())

	setDefaults()
	initSentry()
	initLogger()

	repos := newRepos(postgres.NewClient())
	var s *storage.Client
	var err error
	if viper.GetString("ENV") != "local" {
		s, err = storage.NewClient(ctx)
	} else {
		s, err = storage.NewClient(ctx, option.WithCredentialsFile("./_deploy/service-key.json"))
	}
	if err != nil {
		panic(err)
	}
	ipfsClient := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()
	storageClient := newStorageClient()

	router := gin.Default()

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logger.For(ctx).Info("Registering handlers...")

	t := newThrottler()
	mc := server.NewMultichainProvider(repos, redis.NewCache(redis.CommunitiesDB), rpc.NewEthClient(), http.DefaultClient, ipfsClient, arweaveClient, storageClient, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"))
	mediaQueue := make(chan ProcessMediaInput)
	collectionTokensQueue := make(chan ProcessCollectionTokensRefreshInput)
	go processMedias(mediaQueue, repos.TokenRepository, ipfsClient, arweaveClient, s, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), t)
	go processTokensInCollectionRefreshes(collectionTokensQueue, mc, t)
	return handlersInitServer(router, mediaQueue, collectionTokensQueue, t)
}

func setDefaults() {
	viper.SetDefault("IPFS_URL", "https://gallery.infura-ipfs.io")
	viper.SetDefault("IPFS_API_URL", "https://ipfs.infura.io:5001")
	viper.SetDefault("IPFS_PROJECT_ID", "")
	viper.SetDefault("IPFS_PROJECT_SECRET", "")
	viper.SetDefault("CHAIN", 0)
	viper.SetDefault("ENV", "local")
	viper.SetDefault("GCLOUD_TOKEN_LOGS_BUCKET", "prod-eth-token-logs")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "prod-token-content")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("IMGIX_API_KEY", "")
	viper.SetDefault("CONTRACT_INTERACTION_URL", "https://eth-rinkeby.alchemyapi.io/v2/_2u--i79yarLYdOT4Bgydqa0dBceVRLD")

	if viper.GetString("ENV") != "local" && viper.GetString("SENTRY_DSN") == "" {
		panic("SENTRY_DSN must be set")
	}

	viper.AutomaticEnv()
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

func newStorageClient() *storage.Client {
	var s *storage.Client
	var err error
	if viper.GetString("ENV") != "local" {
		s, err = storage.NewClient(context.Background())
	} else {
		s, err = storage.NewClient(context.Background(), option.WithCredentialsFile("./_deploy/service-key.json"))
	}
	if err != nil {
		logger.For(nil).Errorf("error creating storage client: %v", err)
	}
	return s
}

// configureRootContext configures the main context from which other contexts are derived.
func configureRootContext() context.Context {
	ctx := logger.NewContextWithLogger(context.Background(), logrus.Fields{}, logrus.New())
	logger.For(ctx).Logger.SetReportCaller(true)
	logger.For(ctx).Logger.AddHook(sentryutil.SentryLoggerHook)
	return sentry.SetHubOnContext(ctx, sentry.CurrentHub())
}
