package main

import (
	"context"
	"os"
	"time"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/viper"
)

func main() {
	setDefaults()

	pgClient := postgres.MustCreateClient()

	rows, err := pgClient.Query("select contracts.address,contracts.name from contracts where chain = 0 and (profile_image_url is null or description is null or profile_image_url = '') and is_provider_marked_spam = false order by contracts.last_updated desc;")
	if err != nil {
		panic(err)
	}

	defer rows.Close()

	p := pool.New().WithErrors().WithMaxGoroutines(5)

	for rows.Next() {

		var address, name string

		err := rows.Scan(&address, &name)
		if err != nil {
			panic(err)
		}

		p.Go(func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			logger.For(ctx).Infof("fetching contract %s (%s)", address, name)

			c, err := opensea.FetchContractByAddress(ctx, persist.EthereumAddress(address))
			if err != nil {
				logger.For(ctx).Errorf("error fetching contract %s: %s", address, err)
				return err
			}

			_, err = pgClient.ExecContext(ctx, `update contracts set name = $1, symbol = $2, description = $3, profile_image_url = $4, last_updated = now() where address = $5 and chain = 0;`, c.Collection.Name, c.Symbol, c.Collection.Description, c.Collection.ImageURL, address)
			if err != nil {
				logger.For(ctx).Errorf("error updating contract %s: %s", address, err)
				return err
			}

			logger.For(ctx).Infof("updated contract %s", address)
			return nil
		})

	}

	if err := p.Wait(); err != nil {
		panic(err)
	}

}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("OPENSEA_API_KEY", "")

	viper.AutomaticEnv()

	fi := "local"
	if len(os.Args) > 1 {
		fi = os.Args[1]
	}
	envFile := util.ResolveEnvFile("tokenprocessing", fi)
	util.LoadEncryptedEnvFile(envFile)
}
