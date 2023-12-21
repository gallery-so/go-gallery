package main

import (
	"context"
	"os"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/viper"
)

func main() {

	setDefaults()

	pg := postgres.NewPgxClient()

	queries := coredb.New(pg)

	ctx := context.Background()

	// get every wallet with their owner user ID
	rows, err := pg.Query(ctx, `with tickets as (
    select * from push_notification_tickets
    where created_at > '2023-12-21 16:30:00 +00:00'
      and created_at < '2023-12-21 17:00:00 +00:00'
)
select pnt.id, n.id as notification_id, n.owner_id
from push_notification_tokens pnt
left join tickets on pnt.id = tickets.push_token_id
join users u on pnt.user_id = u.id
left join notifications n on u.id = n.owner_id and n.action = 'Announcement' and n.seen = false
where tickets.id is null
  and not pnt.deleted
  and n.id is not null;
`)
	if err != nil {
		panic(err)
	}

	p := pool.New().WithMaxGoroutines(10).WithErrors()
	t := task.NewClient(ctx)

	for rows.Next() {
		var pushTokenID, notificationID, ownerID persist.DBID

		err := rows.Scan(&pushTokenID, &notificationID, &ownerID)
		if err != nil {
			panic(err)
		}

		p.Go(func() error {
			badgeCount, err := queries.CountUserUnseenNotifications(ctx, ownerID)
			if err != nil {
				return err
			}
			message := task.PushNotificationMessage{
				Title: "Gallery",
				Sound: true,
				Badge: int(badgeCount),
				Data: map[string]any{
					"action":          persist.ActionAnnouncement,
					"notification_id": notificationID,
				},
				PushTokenID: pushTokenID,
				Body:        `Exclusive gift for Gallery members 🎁`,
			}

			logger.For(ctx).Infof("sending push notification: %+v", message)
			return t.CreateTaskForPushNotification(ctx, message)
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
	viper.SetDefault("GCLOUD_SERVICE_KEY_OVERRIDE", "./secrets/prod/service-key.json")

	viper.AutomaticEnv()

	if env.GetString("ENV") != "local" {
		logrus.Info("running in non-local environment, skipping environment configuration")
	} else {
		fi := "local"
		if len(os.Args) > 1 {
			fi = os.Args[1]
		}
		envFile := util.ResolveEnvFile("tokenprocessing", fi)
		util.LoadEncryptedEnvFile(envFile)
	}

}
