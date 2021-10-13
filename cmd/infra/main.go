package main

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/infra"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/persist/mongodb"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

func main() {

	setDefaults()

	events := []infra.EventHash{infra.TransferBatchEventHash, infra.TransferEventHash, infra.TransferSingleEventHash}

	tokenRepo, contractRepo := newRepos()
	indexer := infra.NewIndexer(newEthClient(), newIPFSShell(), tokenRepo, contractRepo, persist.Chain(viper.GetString("CHAIN")), events, "stats.json")

	logrus.Infof("Starting indexer")
	indexer.Start()
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

	ethClient, err := ethclient.DialContext(ctx, viper.GetString("RPC_URL"))
	if err != nil {
		panic(err)
	}
	return ethClient

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
