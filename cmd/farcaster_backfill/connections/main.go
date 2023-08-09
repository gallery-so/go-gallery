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

	// get every farcaster gallery user
	rows, err := pg.Query(ctx, `select u.id,u.pii_socials->'Farcaster'->>'id' from pii.user_view u where u.pii_socials->>'Farcaster' is not null and not u.pii_socials->>'Farcaster' = '' and u.deleted = false and u.universal = false order by last_updated desc;`)
	if err != nil {
		panic(err)
	}

	p := pool.New().WithMaxGoroutines(20).WithErrors()

	for rows.Next() {
		var userID persist.DBID
		var fid string

		err := rows.Scan(&userID, &fid)
		if err != nil {
			panic(err)
		}

		p.Go(func() error {
			logrus.Infof("getting user %s (%s)", userID, fid)
			us, err := neynar.FollowingByUserID(ctx, fid)
			if err != nil {
				logrus.Error(err)
				return nil
			}
			logrus.Infof("got farcaster connection for user %s (%s) len: %d", userID, fid, len(us))
			fids, err := util.Map(us, func(i farcaster.NeynarUser) (string, error) {
				return i.Fid, nil
			})
			gus, err := queries.GetUsersBySocialIDs(ctx, coredb.GetUsersBySocialIDsParams{
				SocialAccountType: "Farcaster",
				SocialIds:         fids,
			})
			logrus.Infof("got gallery user connections for user %s (%s) len: %d", userID, fid, len(gus))

			ids, _ := util.Map(gus, func(i coredb.PiiUserView) (string, error) {
				return persist.GenerateID().String(), nil
			})

			gfeids, _ := util.Map(gus, func(i coredb.PiiUserView) (string, error) {
				return i.ID.String(), nil
			})
			_, err = queries.InsertExternalSocialConnectionsForUser(ctx, coredb.InsertExternalSocialConnectionsForUserParams{
				Ids:               ids,
				SocialAccountType: "Farcaster",
				FollowerID:        userID.String(),
				FolloweeIds:       gfeids,
			})
			if err != nil {
				logrus.Error(err)
				return nil
			}

			return nil
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
