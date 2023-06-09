package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/mikeydub/go-gallery/db/gen/indexerdb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/indexer"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/tracing"
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

	pgx := postgres.NewPgxClient()

	queries := indexerdb.New(pgx)

	var c int
	pgx.QueryRow(ctx, `select count(*) from contracts;`).Scan(&c)
	logrus.Infof("total contracts: %d", c)

	ethClient := rpc.NewEthSocketClient()
	httpClient := &http.Client{Timeout: 10 * time.Minute, Transport: tracing.NewTracingTransport(http.DefaultTransport, false)}

	var rows []indexerdb.Contract
	var err error

	if env.GetString("CLOUD_RUN_JOB") != "" {
		logrus.Infof("running as cloud run job")

		indexer.InitSentry()
		logger.InitWithGCPDefaults()

		jobIndex := env.GetInt("CLOUD_RUN_TASK_INDEX")
		jobCount := env.GetInt("CLOUD_RUN_TASK_COUNT")

		// given the totalTokenCount, and the jobCount, we can calculate the offset and limit for this job
		// we want to evenly distribute the work across the jobs
		// so we can calculate the limit by dividing the totalTokenCount by the jobCount
		// and the offset by multiplying the jobIndex by the limit

		logrus.Infof("jobIndex: %d, jobCount: %d", jobIndex, jobCount)

		r, err := queries.GetReprocessJobRangeByID(ctx, jobIndex)
		if err != nil {
			logrus.Errorf("error getting job range: %v", err)
			panic(err)
		}

		rows, err = queries.GetContractsByIDRange(ctx, indexerdb.GetContractsByIDRangeParams{
			StartID: r.StartID,
			EndID:   r.EndID,
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

		r, err := queries.GetReprocessJobRangeByID(ctx, 0)
		if err != nil {
			logrus.Errorf("error getting job range: %v", err)
			panic(err)
		}
		rows, err = queries.GetContractsByIDRange(ctx, indexerdb.GetContractsByIDRangeParams{
			StartID: r.StartID,
			EndID:   r.EndID,
		})
	}

	if err != nil {
		logrus.Errorf("error getting contracts: %v", err)
		panic(err)
	}

	wp := pool.New().WithMaxGoroutines(50).WithContext(ctx)

	logrus.Infof("processing (%d) contracts...", len(rows))

	asContracts, _ := util.Map(rows, func(r indexerdb.Contract) (persist.Contract, error) {
		return persist.Contract{
			ID:             r.ID,
			Address:        r.Address,
			OwnerMethod:    r.OwnerMethod,
			CreationTime:   persist.CreationTime(r.CreatedAt),
			Deleted:        persist.NullBool(r.Deleted),
			LastUpdated:    persist.LastUpdatedTime(r.LastUpdated),
			Chain:          r.Chain,
			Symbol:         persist.NullString(r.Symbol.String),
			Name:           persist.NullString(r.Name.String),
			OwnerAddress:   r.OwnerAddress,
			CreatorAddress: r.CreatorAddress,
			LatestBlock:    persist.BlockNumber(r.LatestBlock.Int64),
		}, nil
	})

	// group contracts into groups of 100

	groups := util.Chunk(asContracts, 100)

	for i, group := range groups {
		i := i

		wp.Go(func(ctx context.Context) error {
			ctx = sentryutil.NewSentryHubContext(ctx)

			defer func() {
				logger.For(ctx).Infof("finished processing %s", i)
			}()
			results, err := indexer.GetContractMetadatas(ctx, group, httpClient, ethClient)
			if err != nil {
				return err
			}

			errs := []error{}
			for _, result := range results {
				logger.For(ctx).Infof("updating contract %s (%s, %s, %d)", result.Contract.Address, result.Contract.OwnerAddress, result.Contract.CreatorAddress, result.OwnerMethod)
				err = queries.UpdateContractOwnerByID(ctx, indexerdb.UpdateContractOwnerByIDParams{
					OwnerAddress:   result.Contract.OwnerAddress,
					CreatorAddress: result.Contract.CreatorAddress,
					OwnerMethod:    result.OwnerMethod,
					ID:             result.Contract.ID,
				})
				if err != nil {
					errs = append(errs, err)
				}
			}
			if len(errs) > 0 {
				return util.MultiErr(errs)
			}
			return nil
		})
	}

	go func() {
		http.ListenAndServe(":6060", nil)
	}()
	err = wp.Wait()
	logrus.Infof("finished processes with err: %s", err)

}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5433)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("INDEXER_HOST", "http://localhost:6000")
	viper.SetDefault("ALCHEMY_API_URL", "")
	viper.SetDefault("RPC_URL", "https://eth-goerli.g.alchemy.com/v2/_2u--i79yarLYdOT4Bgydqa0dBceVRLD")
	viper.SetDefault("REDIS_URL", "localhost:6379")
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
		envFile := util.ResolveEnvFile("indexer", fi)
		util.LoadEncryptedEnvFile(envFile)
	}
}
