package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gammazero/workerpool"
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
	setDefaults()
	client := &http.Client{
		Timeout: time.Minute * 20,
	}
	pc := postgres.NewClient()

	stmt, err := pc.Prepare(`SELECT id, addresses FROM users WHERE DELETED = FALSE;`)
	if err != nil {
		panic(err)
	}
	validateURL, err := url.Parse("https://indexer-dot-gallery-prod-325303.wl.r.appspot.com/nfts/validate")
	if err != nil {
		panic(err)
	}

	mediaURL, err := url.Parse("https://indexer-dot-gallery-prod-325303.wl.r.appspot.com/media/update")
	if err != nil {
		panic(err)
	}

	users := map[persist.DBID][]persist.Address{}

	res, err := stmt.Query()
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

	wp := workerpool.New(8)
	for u, addrs := range users {
		userID := u
		addresses := addrs
		wp.Submit(func() {
			logrus.Infof("Processing user %s with addresses %v", userID, addresses)
			url := validateURL
			input := indexer.ValidateUsersNFTsInput{
				UserID: userID,
				All:    true,
			}

			b, err := json.Marshal(input)
			if err != nil {
				panic(err)
			}
			resp, err := client.Post(url.String(), "application/json", bytes.NewReader(b))
			if err != nil {
				panic(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				bs, err := io.ReadAll(resp.Body)
				if err != nil {
					panic(err)
				}
				logrus.Errorf("Error validating user %s: %s - %s", userID, string(bs), resp.Status)
			}
			for _, addr := range addresses {
				url = mediaURL
				input := indexer.UpdateMediaInput{
					OwnerAddress: addr,
				}

				b, err := json.Marshal(input)
				if err != nil {
					panic(err)
				}

				resp, err := client.Post(url.String(), "application/json", bytes.NewReader(b))
				if err != nil {
					panic(err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					bs, err := io.ReadAll(resp.Body)
					if err != nil {
						panic(err)
					}
					logrus.Errorf("User %s failed media update with address %s: %s", userID, addr, string(bs))
				}
			}
		})
	}

	go func() {
		for {
			time.Sleep(time.Minute)
			logrus.Infof("Workerpool queue size: %d", wp.WaitingQueueSize())
		}
	}()
	wp.StopWait()

	logrus.Info("Done")

}

func setDefaults() {
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("OPENSEA_API_KEY", "")

	viper.AutomaticEnv()
}
