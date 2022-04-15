package main

import (
	"context"
	"time"

	"cloud.google.com/go/storage"
	"github.com/gammazero/workerpool"
	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/indexer"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/rpc"
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
	tokenRepo := postgres.NewTokenRepository(pc, nil)
	contractRepo := postgres.NewContractRepository(pc)
	ethClient := rpc.NewEthClient()
	ipfsClient := rpc.NewIPFSShell()
	arweaveClient := rpc.NewArweaveClient()
	stg, err := storage.NewClient(context.Background(), option.WithCredentialsFile("./_deploy/service-key.json"))
	if err != nil {
		panic(err)
	}

	stmt, err := pc.Prepare(`SELECT id, addresses FROM users WHERE DELETED = FALSE ORDER BY CREATED_AT ASC;`)
	if err != nil {
		panic(err)
	}

	users := map[persist.DBID][]persist.EthereumAddress{}

	res, err := stmt.Query()
	if err != nil {
		panic(err)
	}

	for res.Next() {
		var id persist.DBID
		var addresses []persist.EthereumAddress

		err := res.Scan(&id, pq.Array(&addresses))
		if err != nil {
			panic(err)
		}
		users[id] = addresses
	}

	if err := res.Err(); err != nil {
		panic(err)
	}
	wp := workerpool.New(6)
	go func() {
		for {
			time.Sleep(time.Minute)
			logrus.Warnf("Workerpool queue size: %d", wp.WaitingQueueSize())
		}
	}()
	for u, addrs := range users {
		userID := u
		addresses := addrs
		f := func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Hour/2)
			go func() {
				defer cancel()
				logrus.Warnf("Processing user %s with addresses %v", userID, addresses)

				for _, addr := range addresses {

					updateInput := indexer.UpdateMediaInput{
						OwnerAddress: addr,
					}
					err = indexer.UpdateMedia(ctx, updateInput, tokenRepo, ethClient, ipfsClient, arweaveClient, stg)
					if err != nil {
						logrus.Errorf("Error processing user %s: %s", userID, err)
					}
					validateInput := indexer.ValidateUsersNFTsInput{
						Wallet: addr,
					}
					_, err := indexer.ValidateNFTs(ctx, validateInput, userRepo, tokenRepo, contractRepo, ethClient, ipfsClient, arweaveClient, stg)
					if err != nil {
						logrus.Errorf("Error processing user %s: %s", userID, err)
					}
				}
			}()
			<-ctx.Done()
			if ctx.Err() != context.Canceled {
				logrus.Errorf("Error processing user %s: %s", userID, ctx.Err())
			}
		}
		wp.Submit(f)
	}

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
