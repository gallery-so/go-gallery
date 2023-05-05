package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	farcaster "github.com/ertan/go-farcaster/pkg"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func init() {
	env.RegisterValidation("FARCASTER_MNEMONIC", "required")
}

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

	apiUrl := "https://api.warpcast.com"
	mnemonic := env.GetString("FARCASTER_MNEMONIC")
	fc := farcaster.NewFarcasterClient(apiUrl, mnemonic, "")

	results := []farcastGalleryAcc{}

	total := 0
	for ; rows.Next(); total++ {
		var address, userID, username string
		err := rows.Scan(&address, &userID, &username)
		if err != nil {
			panic(err)
		}

		logrus.Infof("Checking %s", address)
		u, err := fc.Verifications.GetUserByVerification(address)
		if err != nil {
			logrus.Errorf("Error getting user by verification: %s", err)
			continue
		}

		if u.Fid != 0 || u.Username != "" {
			logrus.Infof("Found %s", username)
			results = append(results, farcastGalleryAcc{
				GalleryUsername: username,
				GalleryId:       userID,
				GalleryAddress:  address,
			})
		}

	}

	asJSON := map[string]interface{}{
		"count":            len(results),
		"out_of":           total,
		"gallery_accounts": results,
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

	if env.GetString("ENV") != "local" {
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
