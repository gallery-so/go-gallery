package main

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/lens"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/viper"
)

func main() {

	setDefaults()

	pg := postgres.NewPgxClient()

	l := lens.NewAPI(&http.Client{Timeout: 10 * time.Second}, redis.NewCache(redis.SocialCache))

	queries := coredb.New(pg)

	ctx := context.Background()

	ctag, err := pg.Exec(ctx, `update contracts set is_provider_marked_spam = true where deleted = false and lower(name) like '%.lens-follower' and chain = 2;`)
	if err != nil {
		panic(err)
	}

	logrus.Infof("marked %d contracts as spam", ctag.RowsAffected())
	var rows pgx.Rows

	if env.GetString("START_ID") != "" {
		// get every wallet with their owner user ID
		rows, err = pg.Query(ctx, `select u.id, w.address from pii.user_view u join wallets w on w.id = any(u.wallets) where u.deleted = false and w.chain = 0 and w.deleted = false and u.universal = false and u.pii_socials->>'Lens' is null and u.id < $1 order by u.created_at desc;`, env.GetString("START_ID"))
		if err != nil {
			panic(err)
		}
	} else {
		// get every wallet with their owner user ID
		rows, err = pg.Query(ctx, `select u.id, w.address from pii.user_view u join wallets w on w.id = any(u.wallets) where u.deleted = false and w.chain = 0 and w.deleted = false and u.universal = false and u.pii_socials->>'Lens' is null order by u.created_at desc;`)
		if err != nil {
			panic(err)
		}
	}

	p := pool.New().WithMaxGoroutines(3).WithErrors()

	for rows.Next() {
		var userID persist.DBID
		var walletAddress persist.Address

		err := rows.Scan(&userID, &walletAddress)
		if err != nil {
			panic(err)
		}

		p.Go(func() error {
			logrus.Infof("getting user %s (%s)", userID, walletAddress)
			u, err := l.DefaultProfileByAddress(ctx, walletAddress)
			if err != nil {
				logrus.Error(err)
				if strings.Contains(err.Error(), "too many requests") {
					time.Sleep(4 * time.Minute)
					u, err = l.DefaultProfileByAddress(ctx, walletAddress)
					if err != nil {
						logrus.Error(err)
						return nil
					}
				} else {
					return nil
				}
			}
			logrus.Infof("got user %s %s %s %s", u.Name, u.Handle, u.Picture.Optimized.URL, u.Bio)
			return queries.AddSocialToUser(ctx, coredb.AddSocialToUserParams{
				UserID: userID,
				Socials: persist.Socials{
					persist.SocialProviderLens: persist.SocialUserIdentifiers{
						Provider: persist.SocialProviderLens,
						ID:       u.ID,
						Display:  true,
						Metadata: map[string]interface{}{
							"username":          u.Handle,
							"name":              util.FirstNonEmptyString(u.Name, u.Handle),
							"profile_image_url": util.FirstNonEmptyString(u.Picture.Optimized.URL, u.Picture.URI),
							"bio":               u.Bio,
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
	viper.SetDefault("START_ID", "")

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
