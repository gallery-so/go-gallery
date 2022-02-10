package indexer

import (
	"context"
	"log"
	"net/http"
	"os"
	"syscall"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/everFinance/goar"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
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

	tokenRepo, contractRepo, userRepo := newRepos()
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
	ipfsClient := newIPFSShell()
	arweaveClient := newArweaveClient()
	ethClient := newEthClient()
	tq := task.NewQueue()

	events := []eventHash{transferBatchEventHash, transferEventHash, transferSingleEventHash}
	i := NewIndexer(ethClient, ipfsClient, arweaveClient, s, tokenRepo, contractRepo, userRepo, persist.Chain(viper.GetString("CHAIN")), events, "stats.json")

	router := gin.Default()

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
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")

	viper.AutomaticEnv()
}

func newEthClient() *ethclient.Client {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dialer := *websocket.DefaultDialer
	dialer.ReadBufferSize = 1024 * 20
	rpcClient, err := rpc.DialWebsocketWithDialer(ctx, viper.GetString("RPC_URL"), "", dialer)
	if err != nil {
		panic(err)
	}

	return ethclient.NewClient(rpcClient)

}

func newIPFSShell() *shell.Shell {
	sh := shell.NewShell(viper.GetString("IPFS_URL"))
	sh.SetTimeout(time.Second * 15)
	return sh
}

func newRepos() (persist.TokenRepository, persist.ContractRepository, persist.UserRepository) {
	pgClient := postgres.NewClient()

	return postgres.NewTokenRepository(pgClient), postgres.NewContractRepository(pgClient), postgres.NewUserRepository(pgClient)
}

func redirectStderr(f *os.File) {
	err := syscall.Dup2(int(f.Fd()), int(os.Stderr.Fd()))
	if err != nil {
		log.Fatalf("Failed to redirect stderr to file: %v", err)
	}
}

func newArweaveClient() *goar.Client {
	return goar.NewClient("https://arweave.net")
}
