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
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/mongodb"
	"github.com/mikeydub/go-gallery/service/persist/multi"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
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

	events := []eventHash{transferBatchEventHash, transferEventHash, transferSingleEventHash}

	tokenRepo, contractRepo, userRepo := newRepos()
	var s *storage.Client
	var err error
	if viper.GetString("ENV") != "local" {
		s, err = storage.NewClient(context.Background())
	} else {
		s, err = storage.NewClient(context.Background(), option.WithCredentialsFile("./decrypted/service-key.json"))
	}
	if err != nil {
		panic(err)
	}
	ipfsClient := newIPFSShell()
	ethClient := newEthClient()
	tq := task.NewQueue()
	i := NewIndexer(ethClient, ipfsClient, s, tokenRepo, contractRepo, userRepo, persist.Chain(viper.GetString("CHAIN")), events, "stats.json")

	router := gin.Default()

	if viper.GetString("ENV") == "local" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logrus.Info("Registering handlers...")
	return handlersInit(router, i, tokenRepo, tq, ethClient, ipfsClient, s), i
}

func setDefaults() {
	viper.SetDefault("RPC_URL", "wss://eth-mainnet.alchemyapi.io/v2/Lxc2B4z57qtwik_KfOS0I476UUUmXT86")
	viper.SetDefault("IPFS_URL", "https://ipfs.io")
	viper.SetDefault("CHAIN", "ETH")
	viper.SetDefault("MONGO_URL", "mongodb://localhost:27017/")
	viper.SetDefault("ENV", "local")
	viper.SetDefault("GCLOUD_TOKEN_LOGS_BUCKET", "eth-token-logs")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "token-content")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("DATABASE", "mongodb")

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
	mgoClient := newMongoClient()
	pgClient := postgres.NewClient()

	switch viper.GetString("DATABASE") {
	case "mongodb":
		return mongodb.NewTokenRepository(mgoClient, nil), mongodb.NewContractRepository(mgoClient), mongodb.NewUserRepository(mgoClient)
	case "postgres":
		return postgres.NewTokenRepository(pgClient), postgres.NewContractRepository(pgClient), postgres.NewUserRepository(pgClient)
	case "multi":
		mongoToken, mongoContract := mongodb.NewTokenRepository(mgoClient, nil), mongodb.NewContractRepository(mgoClient)
		pgToken, pgContract, pgUser := postgres.NewTokenRepository(pgClient), postgres.NewContractRepository(pgClient), postgres.NewUserRepository(pgClient)
		return multi.NewTokenRepository(pgToken, mongoToken), multi.NewContractRepository(pgContract, mongoContract), pgUser
	default:
		panic("Unknown database")
	}
}

func newMongoClient() *mongo.Client {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()
	mgoURL := viper.GetString("MONGO_URL")
	if viper.GetString("ENV") != "local" {
		mongoSecretName := viper.GetString("MONGO_SECRET_NAME")
		secret, err := util.AccessSecret(context.Background(), mongoSecretName)
		if err != nil {
			panic(err)
		}
		mgoURL = string(secret)
	}

	logrus.Infof("Connecting to mongo at %s", mgoURL)

	mOpts := options.Client().ApplyURI(mgoURL)
	mOpts.SetRegistry(mongodb.CustomRegistry)
	mOpts.SetWriteConcern(writeconcern.New(writeconcern.J(true), writeconcern.W(1)))
	mOpts.SetRetryWrites(true)
	mOpts.SetRetryReads(true)

	return mongodb.NewMongoClient(ctx, mOpts)
}

func redirectStderr(f *os.File) {
	err := syscall.Dup2(int(f.Fd()), int(os.Stderr.Fd()))
	if err != nil {
		log.Fatalf("Failed to redirect stderr to file: %v", err)
	}
}
