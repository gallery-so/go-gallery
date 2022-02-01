package main

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func main() {
	setDefaults()
	run()
}

func run() {

	pgClient := postgres.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tokenRepo := postgres.NewTokenRepository(pgClient)
	nftRepo := postgres.NewNFTRepository(pgClient, nil, nil)

	userIDs := getAllUsers(ctx, pgClient)

	usersToNewCollections := getNewCollections(ctx, pgClient, userIDs, nftRepo, tokenRepo)

	updateCollections(ctx, pgClient, usersToNewCollections)
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")

	viper.AutomaticEnv()
}

func updateCollections(ctx context.Context, pgClient *sql.DB, usersToNewCollections map[persist.DBID]map[persist.DBID][]persist.DBID) {
	for _, newCollections := range usersToNewCollections {
		for coll, nfts := range newCollections {
			_, err := pgClient.ExecContext(ctx, `UPDATE collections SET NFTS = $2 WHERE ID = $1`, coll, pq.Array(nfts))
			if err != nil {
				panic(err)
			}
		}
	}
}

func getNewCollections(ctx context.Context, pgClient *sql.DB, userIDs map[persist.DBID][]persist.Address, nftRepo *postgres.NFTRepository, tokenRepo *postgres.TokenRepository) map[persist.DBID]map[persist.DBID][]persist.DBID {
	usersToNewCollections := map[persist.DBID]map[persist.DBID][]persist.DBID{}

	for userID, addresses := range userIDs {
		logrus.Infof("Processing user %s with addresses %v", userID, addresses[0])
		res, err := pgClient.QueryContext(ctx, `SELECT ID, NFTS FROM collections WHERE OWNER_USER_ID = $1`, userID)
		if err != nil {
			panic(err)
		}
		collsToNFTs := map[persist.DBID][]persist.DBID{}
		for res.Next() {
			var nftIDs []persist.DBID
			var collID persist.DBID
			if err = res.Scan(&collID, pq.Array(&nftIDs)); err != nil {
				panic(err)
			}
			collsToNFTs[collID] = nftIDs
		}
		newCollsToNFTs := map[persist.DBID][]persist.DBID{}
		for coll, nftIDs := range collsToNFTs {
			newCollsToNFTs[coll] = make([]persist.DBID, 0, 10)
			for _, nftID := range nftIDs {
				fullNFT, err := nftRepo.GetByID(ctx, nftID)
				if err != nil {
					panic(err)
				}

				tokenEquivelents, err := tokenRepo.GetByTokenIdentifiers(ctx, fullNFT.OpenseaTokenID, fullNFT.Contract.ContractAddress, -1, -1)
				if err != nil {
					panic(err)
				}
				for _, token := range tokenEquivelents {
					if containsAddress(token.OwnerAddress, addresses) {
						logrus.Infof("%s is owned by %s", token.TokenID, token.OwnerAddress)
						newCollsToNFTs[coll] = append(newCollsToNFTs[coll], token.ID)
					}
				}
			}
		}
		usersToNewCollections[userID] = newCollsToNFTs
	}
	return usersToNewCollections
}

func getAllUsers(ctx context.Context, pgClient *sql.DB) map[persist.DBID][]persist.Address {

	res, err := pgClient.QueryContext(ctx, `SELECT ID,ADDRESSES FROM users;`)
	if err != nil {
		panic(err)
	}

	result := map[persist.DBID][]persist.Address{}
	for res.Next() {
		var id persist.DBID
		var addresses []persist.Address
		if err = res.Scan(&id, pq.Array(&addresses)); err != nil {
			panic(err)
		}
		if _, ok := result[id]; !ok {
			result[id] = make([]persist.Address, 0, 3)
		}
		result[id] = append(result[id], addresses...)
	}
	return result
}

func containsAddress(addr persist.Address, addrs []persist.Address) bool {
	for _, a := range addrs {
		if addr.String() == a.String() {
			return true
		}
	}
	return false
}
