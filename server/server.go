package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/everFinance/goar"
	sentry "github.com/getsentry/sentry-go"
	shell "github.com/ipfs/go-ipfs-api"
	magicclient "github.com/magiclabs/magic-admin-go/client"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"

	"cloud.google.com/go/pubsub"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/pubsub/gcp"
	"github.com/mikeydub/go-gallery/service/recommend"
	"github.com/mikeydub/go-gallery/service/recommend/userpref"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/service/rpc/ipfs"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/spf13/viper"
)

func init() {
	env.RegisterValidation("TOKEN_PROCESSING_URL", "required")
	env.RegisterValidation("INDEXER_HOST", "required")
}

// Init initializes the server
func Init() {
	SetDefaults()

	logger.InitWithGCPDefaults()
	initSentry()

	ctx := context.Background()
	c := ClientInit(ctx)
	provider, _ := NewMultichainProvider(ctx, SetDefaults)
	recommender := recommend.NewRecommender(c.Queries)
	p := userpref.NewPersonalization(ctx, c.Queries, c.StorageClient)
	router := CoreInit(ctx, c, provider, recommender, p)
	http.Handle("/", router)
}

type Clients struct {
	Repos           *postgres.Repositories
	Queries         *db.Queries
	HTTPClient      *http.Client
	EthClient       *ethclient.Client
	IPFSClient      *shell.Shell
	ArweaveClient   *goar.Client
	StorageClient   *storage.Client
	TaskClient      *task.Client
	SecretClient    *secretmanager.Client
	PubSubClient    *pubsub.Client
	MagicLinkClient *magicclient.API
	closeFunc       func()
}

func (c *Clients) Close() {
	c.closeFunc()
}

func ClientInit(ctx context.Context) *Clients {
	pq := postgres.MustCreateClient()
	pgx := postgres.NewPgxClient()
	return &Clients{
		Repos:           postgres.NewRepositories(pq, pgx),
		Queries:         db.New(pgx),
		HTTPClient:      &http.Client{Timeout: 0},
		EthClient:       rpc.NewEthClient(),
		IPFSClient:      ipfs.NewShell(),
		ArweaveClient:   rpc.NewArweaveClient(),
		StorageClient:   rpc.NewStorageClient(ctx),
		TaskClient:      task.NewClient(ctx),
		SecretClient:    newSecretsClient(),
		PubSubClient:    gcp.NewClient(ctx),
		MagicLinkClient: auth.NewMagicLinkClient(),
		closeFunc: func() {
			pq.Close()
			pgx.Close()
		},
	}
}

// CoreInit initializes core server functionality. This is abstracted
// so the test server can also utilize it
func CoreInit(ctx context.Context, c *Clients, provider *multichain.Provider, recommender *recommend.Recommender, p *userpref.Personalization) *gin.Engine {
	logger.For(nil).Info("initializing server...")

	if env.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	router := gin.Default()
	router.Use(middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.GinContextToContext(), middleware.ErrLogger())

	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		logger.For(nil).Info("registering validation")
		validate.RegisterCustomValidators(v)
	}

	lock := redis.NewLockClient(redis.NewCache(redis.NotificationLockCache))
	graphqlAPQCache := redis.NewCache(redis.GraphQLAPQCache)
	feedCache := redis.NewCache(redis.FeedCache)
	socialCache := redis.NewCache(redis.SocialCache)
	authRefreshCache := redis.NewCache(redis.AuthTokenForceRefreshCache)

	recommender.Loop(ctx, time.NewTicker(time.Hour))
	p.Loop(ctx, time.NewTicker(time.Minute*15))

	return handlersInit(router, c.Repos, c.Queries, c.HTTPClient, c.EthClient, c.IPFSClient, c.ArweaveClient, c.StorageClient, provider, newThrottler(), c.TaskClient, c.PubSubClient, lock, c.SecretClient, graphqlAPQCache, feedCache, socialCache, authRefreshCache, c.MagicLinkClient, recommender, p)
}

func newSecretsClient() *secretmanager.Client {
	options := []option.ClientOption{}

	if env.GetString("ENV") == "local" {
		fi, err := util.LoadEncryptedServiceKeyOrError("./secrets/dev/service-key-dev.json")
		if err != nil {
			logger.For(nil).WithError(err).Error("error finding service key, running without secrets client")
			return nil
		}
		options = append(options, option.WithCredentialsJSON(fi))
	}

	c, err := secretmanager.NewClient(context.Background(), options...)
	if err != nil {
		panic(fmt.Sprintf("error creating secrets client: %v", err))
	}

	return c
}

func SetDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("REFRESH_JWT_SECRET", "Refresh-Test-Secret")
	viper.SetDefault("REFRESH_JWT_TTL", 60*60*24*90)
	viper.SetDefault("AUTH_JWT_SECRET", "Test-Secret")
	viper.SetDefault("AUTH_JWT_TTL", 60*5)
	viper.SetDefault("ONE_TIME_LOGIN_JWT_SECRET", "One-Time-Login-Test-Secret")
	viper.SetDefault("PORT", 4000)
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("IPFS_URL", "https://gallery.infura-ipfs.io")
	viper.SetDefault("FALLBACK_IPFS_URL", "https://ipfs.io")
	viper.SetDefault("IPFS_API_URL", "https://ipfs.infura.io:5001")
	viper.SetDefault("IPFS_PROJECT_ID", "")
	viper.SetDefault("IPFS_PROJECT_SECRET", "")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "dev-token-content")
	viper.SetDefault("GCLOUD_USER_PREF_BUCKET", "dev-user-pref")
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("PREMIUM_CONTRACT_ADDRESS", "0xe01569ca9b39e55bc7c0dfa09f05fa15cb4c7698=[0,1,2,3,4,5,6,7,8]")
	viper.SetDefault("RPC_URL", "https://eth-goerli.g.alchemy.com/v2/_2u--i79yarLYdOT4Bgydqa0dBceVRLD")
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
	viper.SetDefault("GCLOUD_FEED_QUEUE", "projects/gallery-local/locations/here/queues/feed-event")
	viper.SetDefault("GCLOUD_FEED_BUFFER_SECS", 20)
	viper.SetDefault("FEED_SECRET", "feed-secret")
	viper.SetDefault("TOKEN_PROCESSING_URL", "http://localhost:6500")
	viper.SetDefault("TEZOS_API_URL", "https://api.tzkt.io")
	viper.SetDefault("POAP_API_KEY", "")
	viper.SetDefault("POAP_AUTH_TOKEN", "")
	viper.SetDefault("GAE_VERSION", "")
	viper.SetDefault("TOKEN_PROCESSING_QUEUE", "projects/gallery-local/locations/here/queues/token-processing")
	viper.SetDefault("GOOGLE_CLOUD_PROJECT", "gallery-dev-322005")
	viper.SetDefault("PUBSUB_EMULATOR_HOST", "")
	viper.SetDefault("PUBSUB_TOPIC_NEW_NOTIFICATIONS", "dev-new-notifications")
	viper.SetDefault("PUBSUB_TOPIC_UPDATED_NOTIFICATIONS", "dev-updated-notifications")
	viper.SetDefault("PUBSUB_SUB_NEW_NOTIFICATIONS", "dev-new-notifications-sub")
	viper.SetDefault("PUBSUB_SUB_UPDATED_NOTIFICATIONS", "dev-updated-notifications-sub")
	viper.SetDefault("EMAILS_HOST", "http://localhost:5500")
	viper.SetDefault("RETOOL_AUTH_TOKEN", "TEST_TOKEN")
	viper.SetDefault("BACKEND_SECRET", "BACKEND_SECRET")
	viper.SetDefault("MERCH_CONTRACT_ADDRESS", "0x01f55be815fbd10b1770b008b8960931a30e7f65")
	viper.SetDefault("ETH_PRIVATE_KEY", "")
	viper.SetDefault("FEED_URL", "")
	viper.SetDefault("MAGIC_LINK_SECRET_KEY", "")
	viper.SetDefault("TWITTER_CLIENT_ID", "")
	viper.SetDefault("TWITTER_CLIENT_SECRET", "")
	viper.SetDefault("TWITTER_AUTH_REDIRECT_URI", "http://localhost:3000/auth/twitter")
	viper.SetDefault("FEEDBOT_URL", "")
	viper.SetDefault("GCLOUD_FEEDBOT_TASK_QUEUE", "projects/gallery-local/locations/here/queues/feedbot")
	viper.SetDefault("FEEDBOT_SECRET", "")
	viper.SetDefault("ALCHEMY_API_URL", "")
	viper.SetDefault("ALCHEMY_OPTIMISM_API_URL", "")
	viper.SetDefault("ALCHEMY_POLYGON_API_URL", "")
	viper.SetDefault("ALCHEMY_NFT_API_URL", "")
	viper.SetDefault("INFURA_API_KEY", "")
	viper.SetDefault("INFURA_API_SECRET", "")
	viper.SetDefault("PUSH_NOTIFICATIONS_SECRET", "push-notifications-secret")
	viper.SetDefault("ZORA_API_KEY", "")
	viper.SetDefault("GOLDSKY_API_KEY", "")
	viper.SetDefault("RESERVOIR_API_KEY", "")
	viper.SetDefault("NEYNAR_API_KEY", "")
	viper.SetDefault("EMAILS_QUEUE", "projects/gallery-local/locations/here/queues/email")
	viper.SetDefault("EMAILS_TASK_SECRET", "emails-task-secret")
	viper.SetDefault("AUTOSOCIAL_URL", "")
	viper.SetDefault("AUTOSOCIAL_QUEUE", "")
	viper.SetDefault("AUTOSOCIAL_POLL_QUEUE", "")
	viper.SetDefault("ACTIVITY_QUEUE", "projects/gallery-local/locations/here/queues/activity")

	viper.SetDefault("FARCASTER_MNEMONIC", "")
	viper.SetDefault("FARCASTER_APP_ID", "")

	viper.AutomaticEnv()

	if env.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
	} else {
		fi := "local"
		if len(os.Args) > 1 {
			fi = os.Args[1]
		}
		envFile := util.ResolveEnvFile("backend", fi)
		util.LoadEncryptedEnvFile(envFile)
	}

	if env.GetString("ENV") != "local" {
		util.VarNotSetTo("IMGIX_SECRET", "")
		util.VarNotSetTo("ADMIN_PASS", "TEST_ADMIN_PASS")
		util.VarNotSetTo("SENTRY_DSN", "")
		util.VarNotSetTo("GAE_VERSION", "")
		util.VarNotSetTo("ETH_PRIVATE_KEY", "")
		util.VarNotSetTo("RETOOL_AUTH_TOKEN", "TEST_TOKEN")
		util.VarNotSetTo("BACKEND_SECRET", "BACKEND_SECRET")
		util.VarNotSetTo("PUSH_NOTIFICATIONS_SECRET", "push-notifications-secret")
		util.VarNotSetTo("REFRESH_JWT_SECRET", "Refresh-Test-Secret")
		util.VarNotSetTo("AUTH_JWT_SECRET", "Test-Secret")
		util.VarNotSetTo("ONE_TIME_LOGIN_JWT_SECRET", "One-Time-Login-Test-Secret")
	}
}

func initSentry() {
	if env.GetString("ENV") == "local" {
		logger.For(nil).Info("skipping sentry init")
		return
	}

	logger.For(nil).Info("initializing sentry...")

	err := sentry.Init(sentry.ClientOptions{
		MaxSpans:         100000,
		Dsn:              env.GetString("SENTRY_DSN"),
		Environment:      env.GetString("ENV"),
		TracesSampleRate: env.GetFloat64("SENTRY_TRACES_SAMPLE_RATE"),
		Release:          env.GetString("GAE_VERSION"),
		AttachStacktrace: true,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			event = auth.ScrubEventCookies(event, hint)
			event = sentryutil.UpdateErrorFingerprints(event, hint)
			return event
		},
	})

	if err != nil {
		logger.For(nil).Fatalf("failed to start sentry: %s", err)
	}
}

func newThrottler() *throttle.Locker {
	return throttle.NewThrottleLocker(redis.NewCache(redis.RefreshNFTsThrottleCache), time.Minute*5)
}
