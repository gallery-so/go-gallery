package indexer

import (
	"context"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/persist/mongodb"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

func init() {
	router := coreInit()
	http.Handle("/", router)
}

func coreInit() *gin.Engine {

	setDefaults()

	events := []EventHash{TransferBatchEventHash, TransferEventHash, TransferSingleEventHash}

	tokenRepo, contractRepo := newRepos()
	i := NewIndexer(newEthClient(), newIPFSShell(), tokenRepo, contractRepo, persist.Chain(viper.GetString("CHAIN")), events, "stats.json")

	router := gin.Default()

	logrus.Info("Starting indexer...")
	go i.Start()

	logrus.Info("Registering handlers...")
	return handlersInit(router, i)
}

func handlersInit(router *gin.Engine, i *Indexer) *gin.Engine {
	router.GET("/status", getStatus(i))

	return router
}

func getStatus(i *Indexer) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, gin.H{
			"current_block": i.lastSyncedBlock,
			"recent_block":  i.mostRecentBlock,
			"bad_uris":      i.badURIs,
		})
	}
}

func setDefaults() {
	viper.SetDefault("RPC_URL", "wss://eth-mainnet.alchemyapi.io/v2/Lxc2B4z57qtwik_KfOS0I476UUUmXT86")
	viper.SetDefault("IPFS_URL", "https://ipfs.io")
	viper.SetDefault("CHAIN", "ETH")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "token-content")
	viper.SetDefault("ENV", "local")
	viper.AutomaticEnv()
}

func newEthClient() *ethclient.Client {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
	sh.SetTimeout(time.Second * 2)
	return sh
}

func newRepos() (persist.TokenRepository, persist.ContractRepository) {

	mgoClient := newMongoClient()
	return mongodb.NewTokenMongoRepository(mgoClient), mongodb.NewContractMongoRepository(mgoClient)
}

func newMongoClient() *mongo.Client {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(5)*time.Second)
	defer cancel()
	mgoURL := "mongodb://localhost:27017/"
	if viper.GetString("ENV") != "local" {
		mongoSecretName := viper.GetString("MONGO_SECRET_NAME")
		secret, err := util.AccessSecret(context.Background(), mongoSecretName)
		if err != nil {
			panic(err)
		}
		mgoURL = string(secret)
	}

	mOpts := options.Client().ApplyURI(string(mgoURL))

	mClient, err := mongo.Connect(ctx, mOpts)
	if err != nil {
		panic(err)
	}

	err = mClient.Ping(ctx, readpref.Primary())
	if err != nil {
		panic(err)
	}

	return mClient
}
