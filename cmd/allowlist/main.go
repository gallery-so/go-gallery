package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/gammazero/workerpool"
	"github.com/jackc/pgx/v4"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/spf13/viper"
)

type allowlist []persist.DBID

func main() {

	setDefaults()

	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		fmt.Printf("Took %s", elapsed)
	}()

	pg := postgres.NewPgxClient()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	allowlistFile, err := os.Open("./users_for_allowlist.json")
	if err != nil {
		panic(err)
	}

	var a allowlist
	err = json.NewDecoder(allowlistFile).Decode(&a)
	if err != nil {
		panic(err)
	}

	wallets := make([]persist.Address, 0, len(a))

	wp := workerpool.New(100)
	addrChan := make(chan persist.Address)

	for _, userID := range a {
		user := userID
		wp.Submit(func() {
			var walletAddress persist.Address
			err = pg.QueryRow(ctx, "SELECT w.ADDRESS FROM users u, wallets w WHERE u.ID = $1 AND w.CHAIN = 0 AND w.ID = any(u.WALLETS) ORDER BY array_position(u.WALLETS, w.ID) LIMIT 1", user).Scan(&walletAddress)
			if err != nil && err != pgx.ErrNoRows {
				panic(err)
			}
			addrChan <- walletAddress
		})
	}

	go func() {
		wp.StopWait()
		close(addrChan)
	}()

	for address := range addrChan {
		if address != "" {
			wallets = append(wallets, address)
			logger.For(ctx).Infof("Found wallet address %s", address)
		}
	}

	asJSON, err := json.Marshal(dedupeAddresses(wallets))
	if err != nil {
		panic(err)
	}

	resultFile, err := os.Create("./allowlist.json")
	if err != nil {
		panic(err)
	}

	_, err = resultFile.Write(asJSON)
	if err != nil {
		panic(err)
	}

}

func dedupeAddresses(addresses []persist.Address) []persist.Address {
	seen := make(map[persist.Address]struct{}, len(addresses))
	j := 0
	for _, address := range addresses {
		if _, ok := seen[address]; ok {
			continue
		}
		seen[address] = struct{}{}
		addresses[j] = address
		j++
	}
	return addresses[:j]
}

func setDefaults() {
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")

	viper.AutomaticEnv()
}
