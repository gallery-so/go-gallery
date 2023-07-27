package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/farcaster"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/viper"
)

// run with `go run cmd/notification_prepop/main.go ${some user ID to use as the viewer}`

func main() {

	setDefaults()

	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		fmt.Printf("Took %s", elapsed)
	}()

	pg := postgres.NewPgxClient()

	neynar := farcaster.NewNeynarAPI(&http.Client{Timeout: 10 * time.Second})

	// queries := coredb.New(pg)

	ctx := context.Background()

	// get every wallet with their owner user ID
	rows, err := pg.Query(ctx, `select users.id, wallets.address from users join wallets on wallets.id = any(users.wallets) where users.deleted = false and wallets.chain = 0 order by users.created_at desc;`)
	if err != nil {
		panic(err)
	}

	logrus.Infof("got all wallets and users...")

	p := pool.New().WithMaxGoroutines(10).WithErrors()

	for rows.Next() {
		var userID persist.DBID
		var walletAddress persist.Address

		err := rows.Scan(&userID, &walletAddress)
		if err != nil {
			panic(err)
		}

		p.Go(func() error {
			logrus.Infof("getting user %s (%s)", userID, walletAddress)
			u, err := neynar.UserByAddress(ctx, walletAddress)
			if err != nil {
				logrus.Error(err)
				return nil
			}
			logrus.Infof("got user %s %s %s %s", u.Username, u.DisplayName, u.Pfp.URL, u.Profile.Bio.Text)

			return nil
			// return queries.AddSocialToUser(ctx, coredb.AddSocialToUserParams{
			// 	UserID: userID,
			// 	Socials: persist.Socials{
			// 		persist.SocialProviderFarcaster: persist.SocialUserIdentifiers{
			// 			Provider: persist.SocialProviderFarcaster,
			// 			ID:       u.Fid,
			// 			Display:  true,
			// 			Metadata: map[string]interface{}{
			// 				"username":          u.Username,
			// 				"name":              u.DisplayName,
			// 				"profile_image_url": u.Pfp.URL,
			// 				"bio":               u.Profile.Bio.Text,
			// 			},
			// 		},
			// 	},
			// })

		})

	}

	err = p.Wait()
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
	viper.SetDefault("NEYNAR_API_KEY", "PURPLE")

	viper.AutomaticEnv()

	if env.GetString("ENV") != "local" {
		logrus.Info("running in non-local environment, skipping environment configuration")
	} else {
		fi := "local"
		if len(os.Args) > 1 {
			fi = os.Args[1]
		}
		envFile := util.ResolveEnvFile("backend", fi)
		util.LoadEncryptedEnvFile(envFile)
	}

}
