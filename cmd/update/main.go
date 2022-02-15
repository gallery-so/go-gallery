package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/indexer"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type successOrError struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

func main() {
	client := &http.Client{
		Timeout: time.Minute,
	}
	pc := postgres.NewClient()
	validateURL, err := url.Parse("https://indexer-dot-gallery-prod-325303.wl.r.appspot.com/nfts/validate")
	if err != nil {
		panic(err)
	}

	mediaURL, err := url.Parse("https://indexer-dot-gallery-prod-325303.wl.r.appspot.com/media/update")
	if err != nil {
		panic(err)
	}

	users := map[persist.DBID][]persist.Address{}

	res, err := pc.Query(`SELECT id, address FROM users`)
	if err != nil {
		panic(err)
	}

	for res.Next() {
		var id persist.DBID
		var addresses []persist.Address

		err := res.Scan(&id, pq.Array(&addresses))
		if err != nil {
			panic(err)
		}
		users[id] = addresses
	}

	if err := res.Err(); err != nil {
		panic(err)
	}

	for userID, addresses := range users {
		func() {
			logrus.Infof("Processing user %s with addresses %v", userID, addresses)
			url := validateURL
			url.Query().Set("user_id", userID.String())

			resp, err := client.Get(url.String())
			if err != nil {
				panic(err)
			}
			defer resp.Body.Close()

			res := indexer.ValidateUsersNFTsOutput{}
			if err = json.NewDecoder(resp.Body).Decode(&res); err != nil {
				panic(err)
			}

			if !res.Success {
				logrus.Errorf("User %s failed validation: ", userID, res.Message)
			}

			for _, addr := range addresses {
				url = mediaURL
				url.Query().Set("address", addr.String())

				resp, err := client.Get(url.String())
				if err != nil {
					panic(err)
				}
				defer resp.Body.Close()

				res := successOrError{}
				if err = json.NewDecoder(resp.Body).Decode(&res); err != nil {
					panic(err)
				}
				if !res.Success || res.Error != "" {
					logrus.Errorf("User %s failed media update with address %s: %s", userID, addr, res.Error)
				}
			}
		}()
	}

}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")

	viper.AutomaticEnv()
}
