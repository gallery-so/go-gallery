package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/farcaster"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/viper"
)

func main() {

	setDefaults()

	pg := postgres.NewPgxClient()

	neynar := farcaster.NewNeynarAPI(&http.Client{Timeout: 10 * time.Second})

	queries := coredb.New(pg)

	ctx := context.Background()

	// get every wallet with their owner user ID
	rows, err := pg.Query(ctx, `select u.id, w.address from pii.user_view u join wallets w on w.id = any(u.wallets) where u.deleted = false and w.chain = 0 and w.deleted = false and u.universal = false and u.pii_socials->>'Farcaster' is null order by u.created_at desc;`)
	if err != nil {
		panic(err)
	}

	p := pool.New().WithMaxGoroutines(20).WithErrors()

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

			return queries.AddSocialToUser(ctx, coredb.AddSocialToUserParams{
				UserID: userID,
				Socials: persist.Socials{
					persist.SocialProviderFarcaster: persist.SocialUserIdentifiers{
						Provider: persist.SocialProviderFarcaster,
						ID:       u.Fid.String(),
						Display:  true,
						Metadata: map[string]interface{}{
							"username":          u.Username,
							"name":              u.DisplayName,
							"profile_image_url": u.Pfp.URL,
							"bio":               u.Profile.Bio.Text,
						},
					},
				},
			})

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
	viper.SetDefault("NEYNAR_API_KEY", "")

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
