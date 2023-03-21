package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	farcaster "github.com/ertan/go-farcaster/pkg"
	"github.com/ertan/go-farcaster/pkg/users"
	"github.com/jackc/pgx/v4"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

func init() {
	env.RegisterValidation("FARCASTER_MNEMONIC", "required")
}

type farcastGalleryAcc struct {
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
		fmt.Printf("Took %s\n", elapsed)
	}()

	pg := postgres.NewPgxClient()

	// apiUrl := "https://api.farcaster.xyz"
	apiUrl := "https://api.warpcast.com"
	mnemonic := env.GetString(ctx, "FARCASTER_MNEMONIC")

	fc := farcaster.NewFarcasterClient(apiUrl, mnemonic, "")
	fcUsers := []users.User{}

	cursor := ""
	for {
		fmt.Println("Getting premium users, cursor:", cursor)
		incUsers, nextCur, err := fc.Assets.GetCollectionOwners("gallery-membership-cards", 100, cursor)
		if err != nil {
			panic(err)
		}
		fcUsers = append(fcUsers, incUsers...)
		if nextCur == "" || len(incUsers) < 100 {
			fmt.Println("Done getting premium users:", len(incUsers), nextCur)
			break
		}
		cursor = nextCur
	}

	cursor = ""
	for {
		fmt.Println("Getting general users, cursor:", cursor)
		incUsers, nextCur, err := fc.Assets.GetCollectionOwners("gallerygeneralmembershipcards", 100, cursor)
		if err != nil {
			panic(err)
		}
		fcUsers = append(fcUsers, incUsers...)
		if nextCur == "" || len(incUsers) < 100 {
			fmt.Println("Done getting general users:", len(incUsers), nextCur)
			break
		}
		cursor = nextCur
	}

	fmt.Println("Total fc users:", len(fcUsers))

	existingAddresses := make([]string, 0, len(fcUsers))
	existingUsers := make(map[string]bool)

	for _, fcUser := range fcUsers {

		fmt.Println("Checking user:", fcUser.Username, "fid:", fcUser.Fid)

		verif, err := fc.Verifications.GetVerificationsByFid(fcUser.Fid)
		if err != nil {
			fmt.Println("Error getting verifications for user:", fcUser.Username, "fid:", fcUser.Fid, "err:", err)
			// panic(err)
		}

		for _, v := range verif {

			var userID string
			fmt.Println("address:", v.Address)

			err = pg.QueryRow(ctx, `select id from users where (select id from wallets where address = $1 and chain = 0 and deleted = false limit 1) = any(wallets) and deleted = false;`, strings.ToLower(v.Address)).Scan(&userID)
			if err != nil {
				if err != pgx.ErrNoRows {
					panic(err)
				} else {
					fmt.Println("No user found for address: ", v.Address)
					continue
				}
			}

			existingAddresses = append(existingAddresses, v.Address)
			existingUsers[fcUser.Username] = true

		}

	}

	allUsers := util.MapKeys(existingUsers)

	asMap := map[string]interface{}{
		"total_fc_users":           len(fcUsers),
		"total_existing_addresses": len(existingAddresses),
		"total_existing_users":     len(allUsers),
		"existing_addresses":       existingAddresses,
		"existing_users":           allUsers,
	}

	asJSON, err := json.Marshal(asMap)
	if err != nil {
		panic(err)
	}

	fi, err := os.Create("farcaster_addresses.json")
	if err != nil {
		panic(err)
	}

	_, err = fi.Write(asJSON)
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

	if env.GetString(context.Background(), "ENV") != "local" {
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
