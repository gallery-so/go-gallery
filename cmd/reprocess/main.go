package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
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
	pg := postgres.NewPgxClient()
	clients := server.ClientInit(ctx)

	tp := tokenprocessing.NewTokenProcessor(clients.Queries, clients.EthClient, server.NewMultichainProvider(clients), clients.IPFSClient, clients.ArweaveClient, clients.StorageClient, env.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), clients.Repos.TokenRepository)

	var totalTokenCount int

	err := pg.QueryRow(ctx, `select count(*) from tokens where tokens.deleted = false;`).Scan(&totalTokenCount)
	if err != nil {
		logrus.Errorf("error getting total token count: %v", err)
		panic(err)
	}

	var limit int
	var offset int

	var rows []coredb.GetAllTokensWithContractsRow

	if env.GetString("CLOUD_RUN_JOB") != "" {
		logrus.Infof("running as cloud run job")

		jobIndex := env.GetInt("CLOUD_RUN_TASK_INDEX")
		jobCount := env.GetInt("CLOUD_RUN_TASK_COUNT")

		// given the totalTokenCount, and the jobCount, we can calculate the offset and limit for this job
		// we want to evenly distribute the work across the jobs
		// so we can calculate the limit by dividing the totalTokenCount by the jobCount
		// and the offset by multiplying the jobIndex by the limit

		limit = totalTokenCount / jobCount
		offset = jobIndex * limit

		logrus.Infof("jobIndex: %d, jobCount: %d, totalTokenCount: %d, limit: %d, offset: %d", jobIndex, jobCount, totalTokenCount, limit, offset)

		rows, err = clients.Queries.GetAllTokensWithContracts(ctx, coredb.GetAllTokensWithContractsParams{
			Limit:  int32(limit),
			Offset: int32(offset),
		})
	} else {
		logrus.Infof("running as local job")
		limit = 1000
		offset = 120000
		rows, err = clients.Queries.GetAllTokensWithContracts(ctx, coredb.GetAllTokensWithContractsParams{
			Limit:  int32(limit),
			Offset: int32(offset),
		})
	}

	if err != nil {
		logrus.Errorf("error getting tokens: %v", err)
		panic(err)
	}

	wp := pool.New().WithMaxGoroutines(100).WithContext(ctx)

	logrus.Infof("processing (%d) tokens...", totalTokenCount)

	totalTokens := 0

	for _, row := range rows {

		wallets := []persist.Wallet{}
		for _, w := range row.OwnedByWallets {
			wallets = append(wallets, persist.Wallet{ID: w})
		}
		var userSpam *bool
		if row.IsUserMarkedSpam.Valid {
			b := row.IsUserMarkedSpam.Bool
			userSpam = &b
		}
		var provierSpam *bool
		if row.IsProviderMarkedSpam.Valid {
			b := row.IsProviderMarkedSpam.Bool
			provierSpam = &b
		}

		token := persist.TokenGallery{
			Version:              persist.NullInt32(row.Version.Int32),
			ID:                   row.ID,
			CreationTime:         persist.CreationTime(row.CreatedAt),
			Deleted:              persist.NullBool(row.Deleted),
			LastUpdated:          persist.LastUpdatedTime(row.LastUpdated),
			LastSynced:           persist.LastUpdatedTime(row.LastSynced),
			CollectorsNote:       persist.NullString(row.CollectorsNote.String),
			Media:                row.Media,
			TokenMedia:           row.TokenMediaID,
			FallbackMedia:        row.FallbackMedia,
			TokenType:            persist.TokenType(row.TokenType.String),
			Chain:                row.Chain,
			Name:                 persist.NullString(row.Name.String),
			Description:          persist.NullString(row.Description.String),
			TokenURI:             persist.TokenURI(row.TokenUri.String),
			TokenID:              row.TokenID,
			Quantity:             persist.HexString(row.Quantity.String),
			OwnerUserID:          row.OwnerUserID,
			OwnedByWallets:       wallets,
			OwnershipHistory:     row.OwnershipHistory,
			TokenMetadata:        row.TokenMetadata,
			Contract:             row.Contract,
			ExternalURL:          persist.NullString(row.ExternalUrl.String),
			BlockNumber:          persist.BlockNumber(row.BlockNumber.Int64),
			IsUserMarkedSpam:     userSpam,
			IsProviderMarkedSpam: provierSpam,
		}
		contract := persist.ContractGallery{
			Version:              persist.NullInt32(row.Version_2.Int32),
			ID:                   row.ID_2,
			CreationTime:         persist.CreationTime(row.CreatedAt_2),
			Deleted:              persist.NullBool(row.Deleted_2),
			LastUpdated:          persist.LastUpdatedTime(row.LastUpdated_2),
			Chain:                row.Chain_2,
			Address:              row.Address,
			Symbol:               persist.NullString(row.Symbol.String),
			Name:                 util.ToNullString(row.Name_2.String, true),
			Description:          persist.NullString(row.Description_2.String),
			OwnerAddress:         row.OwnerAddress,
			CreatorAddress:       row.CreatorAddress,
			ProfileImageURL:      persist.NullString(row.ProfileImageUrl.String),
			ProfileBannerURL:     persist.NullString(row.ProfileBannerUrl.String),
			BadgeURL:             persist.NullString(row.BadgeUrl.String),
			IsProviderMarkedSpam: row.IsProviderMarkedSpam_2,
		}
		wp.Go(func(ctx context.Context) error {
			logrus.Infof("processing %s", token.ID)
			defer func() {
				logger.For(ctx).Infof("finished processing %s", token.ID)
			}()
			return tp.ProcessTokenPipeline(ctx, token, contract, "", persist.ProcessingCauseRefresh)
		})
	}

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

	viper.AutomaticEnv()

	if env.GetString("ENV") != "local" {
		logrus.Info("running in non-local environment, skipping environment configuration")
	} else {
		fi := "local"
		if len(os.Args) > 1 {
			fi = os.Args[1]
		}
		envFile := util.ResolveEnvFile("backend", fi)
		util.LoadEncryptedEnvFile(envFile)
	}
}
