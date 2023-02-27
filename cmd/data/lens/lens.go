package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/machinebox/graphql"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

/*
{
  "data": {
    "profile": {
      "id": "0x01",
	}
}
*/

type ResponseStruct struct {
	Profiles struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	} `json:"profile"`

	Error string `json:"error"`
}

type lensGalleryAcc struct {
	galleryUsername string
	galleryId       string
	galleryAddress  string
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

	rows, err := pg.Query(ctx, `select wallets.address,users.id,users.username from users join wallets on wallets.id = any(users.wallets) where wallets.deleted = false and users.deleted = false and wallets.chain = 0;`)
	if err != nil {
		panic(err)
	}

	client := graphql.NewClient("https://api.lens.dev")

	allResults := make([]lensGalleryAcc, 0)

	for rows.Next() {
		var address, userID, username string
		err := rows.Scan(&address, &userID, &username)
		if err != nil {
			panic(err)
		}

		logrus.Infof("Checking %s, currently stored %d", address, len(allResults))
		req := graphql.NewRequest(`
    query ($address: EthereumAddress!) {
        profiles(request:{ownedBy: [$address], limit: 1}) {
			
            	items {
					id
				}
		
        }
    }
`)

		req.Var("address", address)

		var respData ResponseStruct
		if err := client.Run(ctx, req, &respData); err != nil {
			logrus.Errorf("Error getting profile for %s: %s", address, err.Error())
			return
		}

		if respData.Error != "" {
			logrus.Errorf("Error getting profile for %s: %s", address, respData.Error)
			return
		}

		if len(respData.Profiles.Items) > 0 {
			allResults = append(allResults, lensGalleryAcc{
				galleryUsername: username,
				galleryId:       userID,
				galleryAddress:  address,
			})
		}

		time.Sleep(500 * time.Millisecond)

	}

	asJSON := map[string]interface{}{
		"lensGalleries": allResults,
	}

	marshalled, err := json.MarshalIndent(asJSON, "", "  ")
	if err != nil {
		panic(err)
	}

	fi, err := os.Create("lens_galleries.json")
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
