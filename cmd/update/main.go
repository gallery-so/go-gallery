package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gammazero/workerpool"
	"github.com/mikeydub/go-gallery/indexer"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func main() {

	setDefaults()

	start := time.Now()

	pgClient := postgres.NewClient()

	rows, err := pgClient.Query("SELECT ADDRESS FROM wallets WHERE DELETED = false ORDER BY ADDRESS;")
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	addresses := make(chan persist.Address)
	defer close(addresses)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go processAddresses(addresses, wg)

	for rows.Next() {
		var address persist.Address
		err := rows.Scan(&address)
		if err != nil {
			panic(err)
		}
		logrus.Infof("adding address %s", address)
		addresses <- address
	}

	wg.Wait()
	logrus.Infof("Finished in %s", time.Since(start))
}

func processAddresses(addresses <-chan persist.Address, wg *sync.WaitGroup) {
	defer wg.Done()
	wp := workerpool.New(10)
	for address := range addresses {
		a := address
		wp.Submit(func() {
			logrus.Infof("processing address %s", a)
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
			defer cancel()

			body := indexer.UpdateTokenMediaInput{
				OwnerAddress: persist.EthereumAddress(a),
				UpdateAll:    true,
			}
			j, err := json.Marshal(body)
			if err != nil {
				panic(err)
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/nfts/refresh", viper.GetString("INDEXER_HOST")), bytes.NewBuffer(j))
			if err != nil {
				logrus.Errorf("error creating req: %s", err)
				return
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				logrus.Errorf("error doing req: %s", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				respBody := map[string]interface{}{}
				json.NewDecoder(resp.Body).Decode(&respBody)
				logrus.Errorf("%s: %+v", resp.Status, respBody)
			}
		})
	}
	wp.StopWait()
}

func setDefaults() {
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("INDEXER_HOST", "http://localhost:4000")

	viper.AutomaticEnv()
}
