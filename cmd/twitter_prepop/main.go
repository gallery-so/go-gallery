package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// run with `go run cmd/twitter_prepop/main.go ${some user ID to use as the viewer}`

type follower struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

func main() {

	setDefaults()

	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		fmt.Printf("Took %s", elapsed)
	}()

	ownerID := persist.DBID(os.Args[1])

	pg := postgres.NewPgxClient()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	followersFile, err := os.Open("dummy_followers.json")
	if err != nil {
		panic(err)
	}
	defer followersFile.Close()

	res := []follower{}

	err = json.NewDecoder(followersFile).Decode(&res)
	if err != nil {
		panic(err)
	}

	logrus.Infof("Found %d followers", len(res))

	aFewUsers, err := pg.Query(ctx, "SELECT ID FROM USERS WHERE NOT ID = $1 LIMIT $2", ownerID, len(res))
	if err != nil {
		panic(err)
	}

	userIDs := make([]persist.DBID, 0)
	for aFewUsers.Next() {
		var id persist.DBID
		err := aFewUsers.Scan(&id)
		if err != nil {
			panic(err)
		}
		userIDs = append(userIDs, id)
	}

	for i, follower := range res {
		so := persist.Socials{persist.SocialProviderTwitter: persist.SocialUserIdentifiers{
			Provider: persist.SocialProviderTwitter,
			ID:       follower.ID,
			Display:  true,
			Metadata: map[string]interface{}{
				"username": follower.Username,
				"name":     follower.Name,
			},
		}}
		_, err := pg.Exec(ctx, `insert into pii.for_users (user_id, pii_socials) values ($2, $1) on conflict (user_id) where deleted = false do update set pii_socials = for_users.pii_socials || $1;`, so, userIDs[i])
		if err != nil {
			panic(err)
		}
		logrus.Infof("Inserted follower %s", follower.Username)
	}
}

func setDefaults() {
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")

	viper.AutomaticEnv()

}
