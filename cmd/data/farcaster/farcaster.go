package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	farcaster "github.com/ertan/go-farcaster/pkg"
	"github.com/gammazero/workerpool"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

/*
{
  "result": {
    "user": {
      "fid": 17,
      "username": "sds",
      "displayName": "Shane da Silva",
      "pfp": {
        "url": "https://lh3.googleusercontent.com/8gYoPP2mTxWhth4f4NZSQjaIBq0WTQWhwpJB3Cl8YvK3dUwoOLDxCSlUMQrkdM-mb3HNRmY_7xmIxARAEEjgxlXIrgj5nFp3ithB",
        "verified": false
      },
      "profile": {
        "bio": {
          "text": "Building @farcaster",
          "mentions": ['farcaster']
        }
      },
      "followerCount": 1234,
      "followingCount": 567
    }
  }
}
*/

type ResponseStruct struct {
	Result struct {
		User struct {
			Fid      int    `json:"fid"`
			Username string `json:"username"`
		} `json:"user"`
	} `json:"result"`
}

type farcastGalleryAcc struct {
	GalleryUsername string `json:"gallery_username"`
	GalleryId       string `json:"gallery_id"`
	GalleryAddress  string `json:"gallery_address"`
}

func main() {

	ctx := context.Background()

	setDefaults()

	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		fmt.Printf("Took %s", elapsed)
	}()

	pg := postgres.NewPgxClient()

	rows, err := pg.Query(ctx, `select wallets.address,users.id,users.username from users join wallets on wallets.id = any(users.wallets) where wallets.deleted = false and users.deleted = false and wallets.chain = 0 and users.universal = false;`)
	if err != nil {
		panic(err)
	}

	apiUrl := "https://api.farcaster.xyz"
	mnemonic := viper.GetString("FARCASTER_MNEMONIC")
	providerWs := viper.GetString("RPC_URL")
	fc := farcaster.NewFarcasterClient(apiUrl, mnemonic, providerWs)

	results := make(chan farcastGalleryAcc)
	wp := workerpool.New(10)

	total := 0
	for ; rows.Next(); total++ {
		var address, userID, username string
		err := rows.Scan(&address, &userID, &username)
		if err != nil {
			panic(err)
		}

		wp.Submit(func() {

			logrus.Infof("Checking %s", address)
			u, err := fc.Verifications.GetUserByVerification(address)
			if err != nil {
				logrus.Errorf("Error getting user by verification: %s", err)
				return
			}

			if u.Fid != 0 || u.Username != "" {
				results <- farcastGalleryAcc{
					GalleryUsername: username,
					GalleryId:       userID,
					GalleryAddress:  username,
				}
			}
		})
	}

	go func() {
		wp.StopWait()
		close(results)
	}()

	allResults := make([]farcastGalleryAcc, 0, 25)
	for result := range results {
		logrus.Infof("Found %s-%s-%s", result.GalleryUsername, result.GalleryId, result.GalleryAddress)
		allResults = append(allResults, result)
	}

	asJSON := map[string]interface{}{
		"count":            len(allResults),
		"out_of":           total,
		"gallery_accounts": allResults,
	}

	marshalled, err := json.MarshalIndent(asJSON, "", "  ")
	if err != nil {
		panic(err)
	}

	fi, err := os.Create("farcaster_galleries.json")
	if err != nil {
		panic(err)
	}

	_, err = fi.Write(marshalled)
	if err != nil {
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
	viper.SetDefault("FARCASTER_MNEMONIC", "")
	viper.SetDefault("RPC_URL", "")

	viper.AutomaticEnv()

	if viper.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
	} else {
		fi := "local"
		if len(os.Args) > 1 {
			fi = os.Args[1]
		}
		envFile := util.ResolveEnvFile("backend", fi)
		util.LoadEncryptedEnvFile(envFile)
	}
}
