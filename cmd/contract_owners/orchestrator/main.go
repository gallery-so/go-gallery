package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"time"

	_ "net/http/pprof"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	totalJobs = 1000
)

type jobRange struct {
	id    int
	start persist.DBID
	end   persist.DBID
}

func main() {

	setDefaults()

	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		fmt.Printf("Took %s", elapsed)
	}()
	ctx := context.Background()
	pg := postgres.NewPgxClient()

	var totalContractCount int

	err := pg.QueryRow(ctx, `select count(*) from contracts where contracts.deleted = false and (contracts.owner_address is null or contracts.owner_address = '' OR contracts.creator_address IS NULL OR contracts.creator_address = '');`).Scan(&totalContractCount)
	if err != nil {
		logrus.Errorf("error getting total token count: %v", err)
		panic(err)
	}

	limit := int(math.Ceil(float64(totalContractCount) / float64(totalJobs)))

	rows, err := pg.Query(ctx, `select contracts.id from contracts where contracts.deleted = false and (contracts.owner_address is null or contracts.owner_address = '' OR contracts.creator_address IS NULL OR contracts.creator_address = '') order by contracts.id;`)
	if err != nil {
		logrus.Errorf("error getting token ids: %v", err)
		panic(err)
	}

	var curRange jobRange

	for i := 0; rows.Next(); i++ {
		var id persist.DBID
		err := rows.Scan(&id)
		if err != nil {
			logrus.Errorf("error scanning contract id: %v", err)
			panic(err)
		}

		if i%limit == 0 {
			fmt.Printf("starting job range %d (start: %s)\n", i/limit, id)
			curRange = jobRange{
				id:    i / limit,
				start: id,
			}
		}

		if i%limit == limit-1 {
			fmt.Printf("ending job range %d (end: %s)\n", curRange.id, id)
			curRange.end = id
			// repurpose the reprocess table
			_, err = pg.Exec(ctx, `INSERT INTO reprocess_jobs (id, start_id, end_id) VALUES ($1, $2, $3);`, curRange.id, curRange.start, curRange.end)
			if err != nil {
				logrus.Errorf("error inserting job: %v", err)
				panic(err)
			}
		}
	}

	fmt.Printf("Inserted %d jobs with a limit of %d and %d total tokens\n", curRange.id, limit, totalContractCount)
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5433)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")

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
