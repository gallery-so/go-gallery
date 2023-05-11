package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/tokenprocessing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/viper"
)

func main() {

	setDefaults()

	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		fmt.Printf("Took %s", elapsed)
	}()
	ctx := context.Background()
	clients := server.ClientInit(ctx)

	tp := tokenprocessing.NewTokenProcessor(clients.Queries, clients.EthClient, server.NewMultichainProvider(clients), clients.IPFSClient, clients.ArweaveClient, clients.StorageClient, env.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), clients.Repos.TokenRepository)

	var rows []coredb.GetAllTokensWithContractsByIDsRow
	var err error

	if env.GetString("CLOUD_RUN_JOB") != "" {
		logrus.Infof("running as cloud run job")

		tokenprocessing.InitSentry()
		logger.InitWithGCPDefaults()

		jobIndex := env.GetInt("CLOUD_RUN_TASK_INDEX")
		jobCount := env.GetInt("CLOUD_RUN_TASK_COUNT")

		// given the totalTokenCount, and the jobCount, we can calculate the offset and limit for this job
		// we want to evenly distribute the work across the jobs
		// so we can calculate the limit by dividing the totalTokenCount by the jobCount
		// and the offset by multiplying the jobIndex by the limit

		logrus.Infof("jobIndex: %d, jobCount: %d", jobIndex, jobCount)

		r, err := clients.Queries.GetReprocessJobRangeByID(ctx, jobIndex)
		if err != nil {
			logrus.Errorf("error getting job range: %v", err)
			panic(err)
		}

		rows, err = clients.Queries.GetAllTokensWithContractsByIDs(ctx, coredb.GetAllTokensWithContractsByIDsParams{
			StartID: r.TokenStartID,
			EndID:   r.TokenEndID,
		})
	} else {

		logrus.Infof("running as local job")
		logger.SetLoggerOptions(func(logger *logrus.Logger) {
			fi, err := os.Create(fmt.Sprintf("reprocess-%s.log", time.Now().Format("2006-01-02T15-04-05")))
			if err != nil {
				panic(err)
			}
			logger.SetOutput(io.MultiWriter(fi, os.Stdout))
		})

		r, err := clients.Queries.GetReprocessJobRangeByID(ctx, 0)
		if err != nil {
			logrus.Errorf("error getting job range: %v", err)
			panic(err)
		}
		rows, err = clients.Queries.GetAllTokensWithContractsByIDs(ctx, coredb.GetAllTokensWithContractsByIDsParams{
			StartID: r.TokenStartID,
			EndID:   r.TokenEndID,
		})
	}

	if err != nil {
		logrus.Errorf("error getting tokens: %v", err)
		panic(err)
	}

	wp := pool.New().WithMaxGoroutines(25).WithContext(ctx)

	logrus.Infof("processing (%d) tokens...", len(rows))

	totalTokens := 0

	for _, row := range rows {

		if row.IsProviderMarkedSpam.Bool || row.IsUserMarkedSpam.Bool || row.IsProviderMarkedSpam_2 {
			logrus.Infof("skipping token %s because it is marked as spam", row.ID)
			continue
		}

		totalTokens++

		wallets := []persist.Wallet{}
		for _, w := range row.OwnedByWallets {
			wallets = append(wallets, persist.Wallet{ID: w})
		}

		token := persist.TokenGallery{
			Version:          persist.NullInt32(row.Version.Int32),
			ID:               row.ID,
			CreationTime:     persist.CreationTime(row.CreatedAt),
			Deleted:          persist.NullBool(row.Deleted),
			LastUpdated:      persist.LastUpdatedTime(row.LastUpdated),
			LastSynced:       persist.LastUpdatedTime(row.LastSynced),
			CollectorsNote:   persist.NullString(row.CollectorsNote.String),
			Media:            row.Media,
			TokenMedia:       row.TokenMediaID,
			FallbackMedia:    row.FallbackMedia,
			TokenType:        persist.TokenType(row.TokenType.String),
			Chain:            row.Chain,
			Name:             persist.NullString(row.Name.String),
			Description:      persist.NullString(row.Description.String),
			TokenURI:         persist.TokenURI(row.TokenUri.String),
			TokenID:          row.TokenID,
			Quantity:         persist.HexString(row.Quantity.String),
			OwnerUserID:      row.OwnerUserID,
			OwnedByWallets:   wallets,
			OwnershipHistory: row.OwnershipHistory,
			TokenMetadata:    row.TokenMetadata,
			Contract:         row.Contract,
			ExternalURL:      persist.NullString(row.ExternalUrl.String),
			BlockNumber:      persist.BlockNumber(row.BlockNumber.Int64),
		}
		contract := persist.ContractGallery{
			Version:          persist.NullInt32(row.Version_2.Int32),
			ID:               row.ID_2,
			CreationTime:     persist.CreationTime(row.CreatedAt_2),
			Deleted:          persist.NullBool(row.Deleted_2),
			LastUpdated:      persist.LastUpdatedTime(row.LastUpdated_2),
			Chain:            row.Chain_2,
			Address:          row.Address,
			Symbol:           persist.NullString(row.Symbol.String),
			Name:             util.ToNullString(row.Name_2.String, true),
			Description:      persist.NullString(row.Description_2.String),
			OwnerAddress:     row.OwnerAddress,
			CreatorAddress:   row.CreatorAddress,
			ProfileImageURL:  persist.NullString(row.ProfileImageUrl.String),
			ProfileBannerURL: persist.NullString(row.ProfileBannerUrl.String),
			BadgeURL:         persist.NullString(row.BadgeUrl.String),
		}

		anOwner := row.WalletAddress
		wp.Go(func(ctx context.Context) error {
			logrus.Infof("processing %s", token.ID)
			defer func() {
				logger.For(ctx).Infof("finished processing %s", token.ID)
			}()
			return tp.ProcessTokenPipeline(ctx, token, contract, anOwner, persist.ProcessingCauseRefresh)
		})
	}
	go func() {
		http.ListenAndServe(":6060", nil)
	}()
	err = wp.Wait()
	logrus.Infof("finished processes for %d tokens with err: %s", totalTokens, err)

}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("INDEXER_HOST", "http://localhost:6000")
	viper.SetDefault("ALCHEMY_API_URL", "")
	viper.SetDefault("ALCHEMY_OPTIMISM_API_URL", "")
	viper.SetDefault("ALCHEMY_POLYGON_API_URL", "")
	viper.SetDefault("ALCHEMY_NFT_API_URL", "")
	viper.SetDefault("INFURA_API_KEY", "")
	viper.SetDefault("INFURA_API_SECRET", "")
	viper.SetDefault("TEZOS_API_URL", "https://api.tzkt.io")
	viper.SetDefault("POAP_API_KEY", "")
	viper.SetDefault("POAP_AUTH_TOKEN", "")
	viper.SetDefault("OPENSEA_API_KEY", "")
	viper.SetDefault("RPC_URL", "https://eth-goerli.g.alchemy.com/v2/_2u--i79yarLYdOT4Bgydqa0dBceVRLD")
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("IPFS_URL", "https://gallery.infura-ipfs.io")
	viper.SetDefault("FALLBACK_IPFS_URL", "https://ipfs.io")
	viper.SetDefault("IPFS_API_URL", "https://ipfs.infura.io:5001")
	viper.SetDefault("IPFS_PROJECT_ID", "")
	viper.SetDefault("IPFS_PROJECT_SECRET", "")
	viper.SetDefault("IMGIX_API_KEY", "")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "dev-token-content")
	viper.SetDefault("CLOUD_RUN_JOB", "")
	viper.SetDefault("CLOUD_RUN_TASK_INDEX", 0)
	viper.SetDefault("CLOUD_RUN_TASK_COUNT", 1)
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("SENTRY_TRACES_SAMPLE_RATE", 0.2)
	viper.SetDefault("VERSION", "0")

	viper.AutomaticEnv()

	if env.GetString("ENV") != "local" {
		logrus.Info("running in non-local environment, skipping environment configuration")
	} else {
		fi := "local"
		if len(os.Args) > 1 {
			fi = os.Args[1]
		}
		envFile := util.ResolveEnvFile("tokenprocessing", fi)
		util.LoadEncryptedEnvFile(envFile)
	}
}
