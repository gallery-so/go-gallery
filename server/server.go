package server

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/everFinance/goar"
	sentry "github.com/getsentry/sentry-go"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"cloud.google.com/go/pubsub"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/eth"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
	"github.com/mikeydub/go-gallery/service/multichain/poap"
	"github.com/mikeydub/go-gallery/service/multichain/tezos"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/spf13/viper"
)

// Init initializes the server
func Init() {

	setDefaults()

	logger.InitWithGCPDefaults()
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

	err := redis.ClearCache(redis.GalleriesDB)
	if err != nil {
		panic(err)
	}

	repos := newRepos(pqClient, pgx)
	ethClient := newEthClient()
	httpClient := &http.Client{Timeout: 10 * time.Minute}
	ipfsClient := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()
	var storage *storage.Client
	var pub *pubsub.Client
	if viper.GetString("ENV") == "local" {
		storage = media.NewLocalStorageClient(context.Background(), "./_deploy/service-key-dev.json")
		pub, err = pubsub.NewClient(context.Background(), viper.GetString("GOOGLE_CLOUD_PROJECT"), option.WithCredentialsFile("./_deploy/service-key-dev.json"))
		if err != nil {
			panic(err)
		}
	} else {
		storage = media.NewStorageClient(context.Background())
		pub, err = pubsub.NewClient(context.Background(), viper.GetString("GOOGLE_CLOUD_PROJECT"))
		if err != nil {
			panic(err)
		}
	}
	taskClient := task.NewClient(context.Background())
	secretClient := newSecretsClient()
	lock := redis.NewLockClient(redis.NotificationLockDB)

	queries := db.New(pgx)

	return handlersInit(router, repos, queries, ethClient, ipfsClient, arweaveClient, storage, NewMultichainProvider(repos, queries, redis.NewCache(redis.CommunitiesDB), ethClient, httpClient, ipfsClient, arweaveClient, storage, viper.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), taskClient), newThrottler(), taskClient, pub, lock, secretClient)
}

func newSecretsClient() *secretmanager.Client {
	var c *secretmanager.Client
	var err error
	if viper.GetString("ENV") != "local" {
		c, err = secretmanager.NewClient(context.Background())
	} else {
		c, err = secretmanager.NewClient(context.Background(), option.WithCredentialsFile("./_deploy/service-key-dev.json"))
	}
	if err != nil {
		panic(fmt.Sprintf("error creating secrets client: %v", err))
	}
	return c
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
	viper.SetDefault("IPFS_URL", "https://gallery.infura-ipfs.io")
	viper.SetDefault("IPFS_API_URL", "https://ipfs.infura.io:5001")
	viper.SetDefault("IPFS_PROJECT_ID", "")
	viper.SetDefault("IPFS_PROJECT_SECRET", "")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "dev-token-content")
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("GOOGLE_APPLICATION_CREDENTIALS", "_deploy/service-key.json")
	viper.SetDefault("CONTRACT_ADDRESSES", "0x93eC9b03a9C14a530F582aef24a21d7FC88aaC46=[0,1,2,3,4,5,6,7,8]")
	viper.SetDefault("CONTRACT_INTERACTION_URL", "https://eth-goerli.g.alchemy.com/v2/_2u--i79yarLYdOT4Bgydqa0dBceVRLD")
	viper.SetDefault("ADMIN_PASS", "TEST_ADMIN_PASS")
	viper.SetDefault("MIXPANEL_TOKEN", "")
	viper.SetDefault("MIXPANEL_API_URL", "https://api.mixpanel.com/track")
	viper.SetDefault("SIGNUPS_TOPIC", "user-signup")
	viper.SetDefault("ADD_ADDRESS_TOPIC", "user-add-address")
	viper.SetDefault("OPENSEA_API_KEY", "")
	viper.SetDefault("GCLOUD_SERVICE_KEY", "")
	viper.SetDefault("INDEXER_HOST", "http://localhost:6000")
	viper.SetDefault("SNAPSHOT_BUCKET", "gallery-dev-322005.appspot.com")
	viper.SetDefault("TASK_QUEUE_HOST", "")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("GCLOUD_FEED_QUEUE", "projects/gallery-dev-322005/locations/us-west2/queues/feed-event")
	viper.SetDefault("GCLOUD_FEED_BUFFER_SECS", 5)
	viper.SetDefault("FEED_SECRET", "feed-secret")
	viper.SetDefault("TOKEN_PROCESSING_URL", "http://localhost:6500")
	viper.SetDefault("TEZOS_API_URL", "https://api.tzkt.io")
	viper.SetDefault("POAP_API_KEY", "")
	viper.SetDefault("POAP_AUTH_TOKEN", "")
	viper.SetDefault("GAE_VERSION", "")
	viper.SetDefault("TOKEN_PROCESSING_QUEUE", "projects/gallery-dev-322005/locations/us-west2/queues/dev-token-processing")
	viper.SetDefault("GOOGLE_CLOUD_PROJECT", "gallery-dev-322005")
	viper.SetDefault("PUBSUB_TOPIC_NEW_NOTIFICATIONS", "dev-new-notifications")
	viper.SetDefault("PUBSUB_TOPIC_UPDATED_NOTIFICATIONS", "dev-updated-notifications")
	viper.SetDefault("PUBSUB_SUB_NEW_NOTIFICATIONS", "dev-new-notifications-sub")
	viper.SetDefault("PUBSUB_SUB_UPDATED_NOTIFICATIONS", "dev-updated-notifications-sub")
	viper.SetDefault("EMAILS_HOST", "http://localhost:5500")
	viper.SetDefault("RETOOL_AUTH_TOKEN", "TEST_TOKEN")
	viper.SetDefault("BACKEND_SECRET", "BACKEND_SECRET")
	viper.SetDefault("MERCH_CONTRACT_ADDRESS", "0x01f55be815fbd10b1770b008b8960931a30e7f65")

	viper.AutomaticEnv()

	if viper.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
	} else {
		envFile := util.ResolveEnvFile("backend")
		util.LoadEnvFile(envFile)
	}

	util.VarNotSetTo("IMGIX_SECRET", "")
	if viper.GetString("ENV") != "local" {
		util.VarNotSetTo("ADMIN_PASS", "TEST_ADMIN_PASS")
		util.VarNotSetTo("SENTRY_DSN", "")
		util.VarNotSetTo("GAE_VERSION", "")
		util.VarNotSetTo("RETOOL_AUTH_TOKEN", "TEST_TOKEN")
		util.VarNotSetTo("BACKEND_SECRET", "BACKEND_SECRET")
	}
}

func newRepos(pq *sql.DB, pgx *pgxpool.Pool) *postgres.Repositories {
	queries := db.New(pgx)

	return &postgres.Repositories{
		UserRepository:        postgres.NewUserRepository(pq, queries),
		NonceRepository:       postgres.NewNonceRepository(pq, queries),
		TokenRepository:       postgres.NewTokenGalleryRepository(pq, queries),
		CollectionRepository:  postgres.NewCollectionTokenRepository(pq, queries),
		GalleryRepository:     postgres.NewGalleryRepository(queries),
		ContractRepository:    postgres.NewContractGalleryRepository(pq, queries),
		MembershipRepository:  postgres.NewMembershipRepository(pq, queries),
		EarlyAccessRepository: postgres.NewEarlyAccessRepository(pq, queries),
		WalletRepository:      postgres.NewWalletRepository(pq, queries),
		AdmireRepository:      postgres.NewAdmireRepository(queries),
		CommentRepository:     postgres.NewCommentRepository(pq, queries),
	}
}

func newEthClient() *ethclient.Client {
	client, err := ethclient.Dial(viper.GetString("CONTRACT_INTERACTION_URL"))
	if err != nil {
		panic(err)
	}
	return client
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
		Release:          viper.GetString("GAE_VERSION"),
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

func NewMultichainProvider(repos *postgres.Repositories, queries *coredb.Queries, cache *redis.Cache, ethClient *ethclient.Client, httpClient *http.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenBucket string, taskClient *cloudtasks.Client) *multichain.Provider {
	ethChain := persist.ChainETH
	overrides := multichain.ChainOverrideMap{persist.ChainPOAP: &ethChain}
	ethProvider := eth.NewProvider(viper.GetString("INDEXER_HOST"), httpClient, ethClient, taskClient)
	openseaProvider := opensea.NewProvider(ethClient, httpClient)
	tezosProvider := multichain.FallbackProvider{
		Primary:  tezos.NewProvider(viper.GetString("TEZOS_API_URL"), viper.GetString("TOKEN_PROCESSING_URL"), viper.GetString("IPFS_URL"), httpClient, ipfsClient, arweaveClient, storageClient, tokenBucket),
		Fallback: tezos.NewObjktProvider(viper.GetString("IPFS_URL")),
		Eval: func(ctx context.Context, token multichain.ChainAgnosticToken) bool {
			return tezos.IsSigned(ctx, token) && tezos.ContainsTezosKeywords(ctx, token)
		},
	}
	poapProvider := poap.NewProvider(httpClient, viper.GetString("POAP_API_KEY"), viper.GetString("POAP_AUTH_TOKEN"))
	return multichain.NewProvider(context.Background(), repos, queries, cache, taskClient,
		overrides,
		ethProvider,
		openseaProvider,
		tezosProvider,
		poapProvider,
	)
}

func newThrottler() *throttle.Locker {
	return throttle.NewThrottleLocker(redis.NewCache(redis.RefreshNFTsThrottleDB), time.Minute*5)
}
