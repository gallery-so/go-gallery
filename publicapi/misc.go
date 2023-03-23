package publicapi

import (
	"context"
	"encoding/json"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/persist/postgres"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
)

type MiscAPI struct {
	repos         *postgres.Repositories
	queries       *db.Queries
	loaders       *dataloader.Loaders
	validator     *validator.Validate
	ethClient     *ethclient.Client
	storageClient *storage.Client
}

func (api MiscAPI) GetGeneralAllowlist(ctx context.Context) ([]persist.EthereumAddress, error) {
	// Nothing to validate

	bucket := env.GetString("SNAPSHOT_BUCKET")
	logger.For(ctx).Infof("Proxying snapshot from bucket %s", bucket)

	obj := api.storageClient.Bucket(env.GetString("SNAPSHOT_BUCKET")).Object("snapshot.json")

	r, err := obj.NewReader(ctx)
	if err != nil {
		return nil, err
	}

	var addresses []persist.EthereumAddress
	err = json.NewDecoder(r).Decode(&addresses)
	if err != nil {
		return nil, err
	}

	err = r.Close()
	if err != nil {
		return nil, err
	}

	return addresses, nil
}

func (api MiscAPI) GetGalleryOfTheWeekWinners(ctx context.Context) ([]db.User, error) {
	// hard-coded for now
	winnerUserIds := []persist.DBID{
		// hamsun
		"22e1Kq9LQS2W75wBeZET1MVXsOQ",
		// aboutblank
		"24125QyTxCe72VqhKweRGFnBJl5",
		// the_ayybee_gallery
		"22RiP4IC3D0bLgwZxZebwMG5Y3m",
		// duane king
		"27zUnqpUL5YBc8cB2a6fPhGg5Mu",
		// casesimmons
		"22tlEnbSpJ38BqD9xoggxnkhNip",
		// laury
		"25XXRXw1B0y65xBo5ghGqRTW9Pt",
		// walt
		"29cvJYtKfauXyNZMeKYb34csdws",
		// salt
		"29oheBA67Mv3Rs6m7z8jEdK0ALs",
	}

	possibleWinners, errors := api.loaders.UserByUserID.LoadAll(winnerUserIds)

	winners := []db.User{}
	for i, err := range errors {
		if err == nil {
			winners = append(winners, possibleWinners[i])
		}
	}

	return winners, nil
}
