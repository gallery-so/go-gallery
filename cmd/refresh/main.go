package main

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/gammazero/workerpool"
	"github.com/mikeydub/go-gallery/admin"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func main() {
	setDefaults()

	logrus.SetLevel(logrus.DebugLevel)

	refreshFile, err := os.Open("refresh.json")
	if err != nil {
		panic(err)
	}

	var allAddresses []persist.Address

	if err = json.NewDecoder(refreshFile).Decode(&allAddresses); err != nil {
		panic(err)
	}

	wp := workerpool.New(3)

	groupings := [][]persist.Address{}

	for i := 0; i < len(allAddresses); i += 100 {
		if i+100 < len(allAddresses) {
			groupings = append(groupings, allAddresses[i:i+100])
		} else {
			groupings = append(groupings, allAddresses[i:])
		}
	}

	pc := postgres.NewClient()

	galleryRepo := postgres.NewGalleryRepository(pc, nil)
	userRepo := postgres.NewUserRepository(pc)
	nftRepo := postgres.NewNFTRepository(pc, galleryRepo)
	collRepo := postgres.NewCollectionRepository(pc, galleryRepo)
	for _, group := range groupings {
		g := group
		wp.Submit(func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Hour/2)
			go func() {
				defer cancel()
				err = admin.RefreshOpensea(ctx, admin.RefreshNFTsInput{Addresses: g}, userRepo, nftRepo, collRepo)
				if err != nil {
					logrus.Errorf("Error refreshing opensea: %s", err)
				}
			}()
			<-ctx.Done()

			if ctx.Err() != context.Canceled {
				logrus.Errorf("Error refreshing opensea: %s", ctx.Err())
			}
		})

	}

	wp.StopWait()

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
