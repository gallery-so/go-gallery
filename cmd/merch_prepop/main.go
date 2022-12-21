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

type discountCodes struct {
	TShirts []string `json:"t_shirts"`
	Hats    []string `json:"hats"`
	Cards   []string `json:"cards"`
}

func main() {

	setDefaults()

	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		fmt.Printf("Took %s", elapsed)
	}()

	pg := postgres.NewPgxClient()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	discountCodeFile, err := os.Open("./discount_codes.json")
	if err != nil {
		panic(err)
	}

	var discountCodes discountCodes
	err = json.NewDecoder(discountCodeFile).Decode(&discountCodes)
	if err != nil {
		panic(err)
	}

	for _, code := range discountCodes.TShirts {
		_, err = pg.Exec(ctx, "INSERT INTO merch (id, discount_code, object_type) VALUES ($1, $2, $3)", persist.GenerateID(), code, 0)
		if err != nil {
			panic(err)
		}
		fmt.Printf("Inserted %s for t-shirts\n", code)
	}

	for _, code := range discountCodes.Hats {
		_, err = pg.Exec(ctx, "INSERT INTO merch (id, discount_code, object_type) VALUES ($1, $2, $3)", persist.GenerateID(), code, 1)
		if err != nil {
			panic(err)
		}
		fmt.Printf("Inserted %s for hats\n", code)
	}

	for _, code := range discountCodes.Cards {
		_, err = pg.Exec(ctx, "INSERT INTO merch (id, discount_code, object_type) VALUES ($1, $2, $3)", persist.GenerateID(), code, 2)
		if err != nil {
			panic(err)
		}
		fmt.Printf("Inserted %s for cards\n", code)
	}

}

func setDefaults() {
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")

	viper.SetDefault("POSTGRES_SERVER_CA", "")
	viper.SetDefault("POSTGRES_CLIENT_CERT", "")
	viper.SetDefault("POSTGRES_CLIENT_KEY", "")

	viper.AutomaticEnv()
}
