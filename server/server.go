package server

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	sentry "github.com/getsentry/sentry-go"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/site"
	"github.com/mikeydub/go-gallery/validate"
	"github.com/spf13/viper"
	"google.golang.org/api/option"
)

// Init initializes the server
func Init() {
	setDefaults()

	initLogger()
	initSentry()

	router := CoreInit(postgres.NewClient(), postgres.NewPgxClient())

	http.Handle("/", router)
}

// CoreInit initializes core server functionality. This is abstracted
// so the test server can also utilize it
func CoreInit(pqClient *sql.DB, pgx *pgxpool.Pool) *gin.Engine {
	logger.For(nil).Info("initializing server...")

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	router := gin.Default()
	router.Use(middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.GinContextToContext(), middleware.ErrLogger())

	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		logger.For(nil).Info("registering validation")
		validate.RegisterCustomValidators(v)
	}

	if err := redis.ClearCache(redis.GalleriesDB); err != nil {
		panic(err)
	}

	repos := newRepos(pqClient)
	ethClient := newEthClient()
	httpClient := &http.Client{Timeout: 10 * time.Second}
	ipfsClient := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()

	return handlersInit(router, repos, sqlc.New(pgx), ethClient, ipfsClient, arweaveClient, newStorageClient(), newMultichainProvider(repos, ethClient, httpClient))
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

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("JWT_SECRET", "Test-Secret")
	viper.SetDefault("JWT_TTL", 60*60*24*14)
	viper.SetDefault("PORT", 4000)
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("IPFS_URL", "https://ipfs.io")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "token-content")
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("GOOGLE_APPLICATION_CREDENTIALS", "_deploy/service-key.json")
	viper.SetDefault("CONTRACT_ADDRESSES", "0x93eC9b03a9C14a530F582aef24a21d7FC88aaC46=[0,1,2,3,4,5,6,7,8]")
	viper.SetDefault("CONTRACT_INTERACTION_URL", "https://eth-rinkeby.alchemyapi.io/v2/_2u--i79yarLYdOT4Bgydqa0dBceVRLD")
	viper.SetDefault("ADMIN_PASS", "TEST_ADMIN_PASS")
	viper.SetDefault("MIXPANEL_TOKEN", "")
	viper.SetDefault("MIXPANEL_API_URL", "https://api.mixpanel.com/track")
	viper.SetDefault("SIGNUPS_TOPIC", "user-signup")
	viper.SetDefault("ADD_ADDRESS_TOPIC", "user-add-address")
	viper.SetDefault("OPENSEA_API_KEY", "")
	viper.SetDefault("GCLOUD_SERVICE_KEY", "")
	viper.SetDefault("INDEXER_HOST", "http://localhost:4000")
	viper.SetDefault("SNAPSHOT_BUCKET", "gallery-dev-322005.appspot.com")
	viper.SetDefault("TASK_QUEUE_HOST", "localhost:8123")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("GCLOUD_FEED_QUEUE", "projects/gallery-local/locations/here/queues/feed-event")
	viper.SetDefault("GCLOUD_FEED_BUFFER_SECS", 5)
	viper.SetDefault("FEED_SECRET", "feed-secret")

	viper.AutomaticEnv()

	if viper.GetString("ENV") == "local" {
		// Tests can run from directories deeper in the source tree, so we need to search parent directories to find this config file
		path, err := util.FindFile("_internal/app-local-backend.yaml", 3)
		if err != nil {
			panic(err)
		}

		viper.SetConfigFile(path)
		if err := viper.ReadInConfig(); err != nil {
			panic(fmt.Sprintf("error reading viper config: %s\nmake sure your _internal directory is decrypted and up-to-date", err))
		}
	}

	if viper.GetString("ENV") != "local" && viper.GetString("ADMIN_PASS") == "TEST_ADMIN_PASS" {
		panic("ADMIN_PASS must be set")
	}

	if viper.GetString("ENV") != "local" && viper.GetString("SENTRY_DSN") == "" {
		panic("SENTRY_DSN must be set")
	}

	if viper.GetString("IMGIX_SECRET") == "" {
		panic("IMGIX_SECRET must be set")
	}
}

func newRepos(db *sql.DB) *persist.Repositories {
	galleriesCacheToken := redis.NewCache(1)
	galleryTokenRepo := postgres.NewGalleryTokenRepository(db, galleriesCacheToken)

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
		CommunityRepository:   postgres.NewCommunityTokenRepository(db, redis.NewCache(redis.CommunitiesDB)),
		EarlyAccessRepository: postgres.NewEarlyAccessRepository(db),
		WalletRepository:      postgres.NewWalletRepository(db),
	}
}

func newEthClient() *ethclient.Client {
	client, err := ethclient.Dial(viper.GetString("CONTRACT_INTERACTION_URL"))
	if err != nil {
		panic(err)
	}
	return client
}

func initLogger() {
	logger.SetLoggerOptions(func(logger *logrus.Logger) {
		logger.SetReportCaller(true)

		if viper.GetString("ENV") != "production" {
			logger.SetLevel(logrus.DebugLevel)
		}

		if viper.GetString("ENV") == "local" {
			logger.SetFormatter(&logrus.TextFormatter{DisableQuote: true})
		} else {
			// Use a JSONFormatter for non-local environments because Google Cloud Logging works well with JSON-formatted log entries
			logger.SetFormatter(&logrus.JSONFormatter{})
		}
	})
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
			return event
		},
	})

	if err != nil {
		logger.For(nil).Fatalf("failed to start sentry: %s", err)
	}
}

func newMultichainProvider(repos *persist.Repositories, ethClient *ethclient.Client, httpClient *http.Client) *multichain.Provider {
	return multichain.NewMultiChainDataRetriever(context.Background(), repos.TokenRepository, repos.ContractRepository, repos.UserRepository, opensea.NewProvider(ethClient, httpClient))
}

func newFigure31Integration(ctx context.Context, repos *persist.Repositories, ethClient *ethclient.Client, httpClient *http.Client, pgx *pgxpool.Pool) *site.Figure31Integration {
	if viper.GetString("FIGURE31_USER_ID") == "" {
		panic("FIGURE31_USER_ID is not set")
	}

	if viper.GetString("FIGURE31_COLLECTION_ID") == "" {
		panic("FIGURE31_COLLECTION_ID is not set")
	}

	if viper.GetString("FIGURE31_CONTRACT_ADDRESS") == "" {
		panic("FIGURE31_CONTRACT_ADDRESS is not set")
	}

	if viper.GetString("FIGURE31_ADDRESS") == "" {
		panic("FIGURE31_ADDRESS is not set")
	}

	if viper.GetInt("FIGURE31_COLUMN_COUNT") == 0 {
		panic("FIGURE31_COLUMN_COUNT is not set")
	}

	if viper.GetInt("FIGURE31_COLLECTION_SIZE") == 0 {
		panic("FIGURE31_COLLECTION_SIZE is not set")
	}

	return site.NewFigure31Integration(
		dataloader.NewLoaders(ctx, sqlc.New(pgx)),
		newMultichainProvider(repos, ethClient, httpClient),
		repos,
		pgx,
		site.Figure31IntegrationInput{
			UserID:         persist.DBID(viper.GetString("FIGURE31_USER_ID")),
			CollectionID:   persist.DBID(viper.GetString("FIGURE31_COLLECTION_ID")),
			ContractAddr:   viper.GetString("FIGURE31_CONTRACT_ADDRESS"),
			ArtistAddr:     viper.GetString("FIGURE31_ADDRESS"),
			ColumnCount:    viper.GetInt("FIGURE31_COLUMN_COUNT"),
			CollectionSize: viper.GetInt("FIGURE31_COLLECTION_SIZE"),
		},
	)
}
