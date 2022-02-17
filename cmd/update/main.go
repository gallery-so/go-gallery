package main

import (
	"context"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/everFinance/goar"
	"github.com/gammazero/workerpool"
	"github.com/gorilla/websocket"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/indexer"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/api/option"
)

type successOrError struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

func main() {
	setDefaults()

	pc := postgres.NewClient()

	userRepo := postgres.NewUserRepository(pc)
	tokenRepo := postgres.NewTokenRepository(pc)
	contractRepo := postgres.NewContractRepository(pc)
	ethClient := newEthClient()
	ipfsClient := newIPFSShell()
	arweaveClient := newArweaveClient()
	stg, err := storage.NewClient(context.Background(), option.WithCredentialsFile("./_deploy/service-key.json"))
	if err != nil {
		panic(err)
	}

	stmt, err := pc.Prepare(`SELECT id, addresses FROM users WHERE DELETED = FALSE;`)
	if err != nil {
		panic(err)
	}

	users := map[persist.DBID][]persist.Address{}

	res, err := stmt.Query()
	if err != nil {
		panic(err)
	}

	for res.Next() {
		var id persist.DBID
		var addresses []persist.Address

		err := res.Scan(&id, pq.Array(&addresses))
		if err != nil {
			panic(err)
		}
		users[id] = addresses
	}

	if err := res.Err(); err != nil {
		panic(err)
	}
	wp := workerpool.New(10)
	for u, addrs := range users {
		userID := u
		addresses := addrs
		wp.Submit(func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
			defer cancel()
			logrus.Warnf("Processing user %s with addresses %v", userID, addresses)

			input := indexer.ValidateUsersNFTsInput{
				UserID: userID,
			}

			_, err := indexer.ValidateNFTs(ctx, input, userRepo, tokenRepo, contractRepo, ethClient, ipfsClient, arweaveClient, stg)
			if err != nil {
				logrus.Errorf("Error processing user %s: %s", userID, err)
			}
			for _, addr := range addresses {

				input := indexer.UpdateMediaInput{
					OwnerAddress: addr,
				}

				err = indexer.UpdateMedia(ctx, input, tokenRepo, ethClient, ipfsClient, arweaveClient, stg)
				if err != nil {
					logrus.Errorf("Error processing user %s: %s", userID, err)
				}
			}
		})
	}

	go func() {
		for {
			time.Sleep(time.Minute)
			logrus.Warnf("Workerpool queue size: %d", wp.WaitingQueueSize())
		}
	}()
	wp.StopWait()

	logrus.Info("Done")

}

func setDefaults() {
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("OPENSEA_API_KEY", "")
	viper.SetDefault("RPC_URL", "wss://eth-mainnet.alchemyapi.io/v2/Lxc2B4z57qtwik_KfOS0I476UUUmXT86")
	viper.SetDefault("IPFS_URL", "https://ipfs.io")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "token-content")

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

func newArweaveClient() *goar.Client {
	return goar.NewClient("https://arweave.net")
}
