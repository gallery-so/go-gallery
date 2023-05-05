package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/spf13/viper"
)

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

	rows, err := pg.Query(ctx, "SELECT w.ADDRESS FROM pii.user_view u, wallets w WHERE NOT u.pii_socials->>'Twitter' = '' AND w.CHAIN = 0 AND w.ID = any(u.WALLETS) ORDER BY array_position(u.WALLETS, w.ID);")
	if err != nil {
		panic(err)
	}

	var wallets []persist.Address
	for rows.Next() {
		var address persist.Address
		err = rows.Scan(&address)
		if err != nil {
			panic(err)
		}
		wallets = append(wallets, address)
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
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")

	viper.AutomaticEnv()
}
