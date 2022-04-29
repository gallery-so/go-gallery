package indexer

import (
	"context"
	"net/http"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/service/task"
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

func coreInit() (*gin.Engine, *Indexer) {

	setDefaults()

	tokenRepo, contractRepo, userRepo, collRepo := newRepos()
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
	tq := task.NewQueue()

	events := []eventHash{transferBatchEventHash, transferEventHash, transferSingleEventHash, foundationMintedEventHash, foundationTransferEventHash}
	i := NewIndexer(ethClient, ipfsClient, arweaveClient, s, tokenRepo, contractRepo, userRepo, collRepo, persist.Chain(viper.GetInt("CHAIN")), events)

	router := gin.Default()

	router.Use(middleware.HandleCORS())

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logrus.Info("Registering handlers...")
	return handlersInit(router, i, tokenRepo, contractRepo, userRepo, tq, ethClient, ipfsClient, arweaveClient, s), i
}

func setDefaults() {
	viper.SetDefault("RPC_URL", "wss://eth-mainnet.alchemyapi.io/v2/Lxc2B4z57qtwik_KfOS0I476UUUmXT86")
	viper.SetDefault("IPFS_URL", "https://ipfs.io")
	viper.SetDefault("CHAIN", "ETH")
	viper.SetDefault("ENV", "local")
	viper.SetDefault("GCLOUD_TOKEN_LOGS_BUCKET", "eth-token-logs")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "token-content")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5433)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")

	viper.AutomaticEnv()
}

func newRepos() (persist.TokenRepository, persist.ContractRepository, persist.UserRepository, persist.CollectionTokenRepository) {
	pgClient := postgres.NewClient()
	galleryRepo := postgres.NewGalleryTokenRepository(pgClient, nil)
	return postgres.NewTokenRepository(pgClient, galleryRepo), postgres.NewContractRepository(pgClient), postgres.NewUserRepository(pgClient), postgres.NewCollectionTokenRepository(pgClient, galleryRepo)
}
