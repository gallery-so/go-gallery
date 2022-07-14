package indexer

import (
	"context"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/api/option"
)

// Init initializes the indexer
func Init() {
	router, i := coreInit()
	logrus.Info("Starting indexer...")
	go i.Start()
	http.Handle("/", router)
}

// InitServer initializes the indexer server
func InitServer() {
	router := coreInitServer()
	logrus.Info("Starting indexer server...")
	http.Handle("/", router)
}

func coreInit() (*gin.Engine, *indexer) {

	setDefaults()

	tokenRepo, contractRepo := newRepos()
	var s *storage.Client
	var err error
	if viper.GetString("ENV") != "local" {
		s, err = storage.NewClient(context.Background())
	} else {
		s, err = storage.NewClient(context.Background(), option.WithCredentialsFile("./_deploy/service-key.json"))
	}
	if err != nil {
		panic(err)
	}
	ethClient := rpc.NewEthClient()
	ipfsClient := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()

	events := []eventHash{transferBatchEventHash, transferEventHash, transferSingleEventHash}
	i := newIndexer(ethClient, ipfsClient, arweaveClient, s, tokenRepo, contractRepo, persist.Chain(viper.GetInt("CHAIN")), events)

	router := gin.Default()

	router.Use(middleware.HandleCORS())

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logrus.Info("Registering handlers...")
	return handlersInit(router, i, tokenRepo, contractRepo, ethClient, ipfsClient, arweaveClient, s), i
}

func coreInitServer() *gin.Engine {

	setDefaults()

	tokenRepo, contractRepo := newRepos()
	var s *storage.Client
	var err error
	if viper.GetString("ENV") != "local" {
		s, err = storage.NewClient(context.Background())
	} else {
		s, err = storage.NewClient(context.Background(), option.WithCredentialsFile("./_deploy/service-key.json"))
	}
	if err != nil {
		panic(err)
	}
	ethClient := rpc.NewEthClient()
	ipfsClient := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()

	router := gin.Default()

	router.Use(middleware.HandleCORS())

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logrus.Info("Registering handlers...")

	queueChan := make(chan processTokensInput)

	t := newThrottler()

	go processMedialessTokens(queueChan, tokenRepo, contractRepo, ipfsClient, ethClient, arweaveClient, s, t)
	return handlersInitServer(router, queueChan, tokenRepo, contractRepo, ethClient, ipfsClient, arweaveClient, s)
}

func setDefaults() {
	viper.SetDefault("RPC_URL", "wss://eth-mainnet.alchemyapi.io/v2/Lxc2B4z57qtwik_KfOS0I476UUUmXT86")
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

	viper.AutomaticEnv()
}

func newRepos() (persist.TokenRepository, persist.ContractRepository) {
	pgClient := postgres.NewClient()
	return postgres.NewTokenRepository(pgClient), postgres.NewContractRepository(pgClient)
}

func newThrottler() *throttle.Locker {
	return throttle.NewThrottleLocker(redis.NewCache(redis.IndexerServerThrottleDB), time.Minute*5)
}
