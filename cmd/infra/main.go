package main

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/infra"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func main() {

	setDefaults()

	events := []infra.EventHash{infra.TransferBatchEventHash, infra.TransferEventHash, infra.TransferSingleEventHash}

	indexer := infra.NewIndexer(newEthClient(), newIPFSShell(), persist.Chain(viper.GetString("CHAIN")), events, "stats.json")

	logrus.Infof("Starting indexer")
	indexer.Start()
}

func setDefaults() {
	viper.SetDefault("RPC_URL", "wss://eth-mainnet.alchemyapi.io/v2/Lxc2B4z57qtwik_KfOS0I476UUUmXT86")
	viper.SetDefault("IPFS_URL", "https://ipfs.io")
	viper.SetDefault("CHAIN", "ETH")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "token-content")
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
