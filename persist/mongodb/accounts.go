package mongodb

import (
	"context"
	"strings"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const accountCollName = "accounts"

// AccountMongoRepository is a repository for storing authentication nonces in a MongoDB database
type AccountMongoRepository struct {
	accountStorage *storage
}

// NewAccountMongoRepository returns a new instance of a login attempt repository
func NewAccountMongoRepository(mgoClient *mongo.Client) *AccountMongoRepository {
	return &AccountMongoRepository{
		accountStorage: newStorage(mgoClient, 0, galleryDBName, accountCollName),
	}
}

// UpsertByAddress upserts an account by a given address
// pUpdate represents a struct with bson tags to specify which fields to update
func (a *AccountMongoRepository) UpsertByAddress(pCtx context.Context, pAddress persist.Address, pUpsert *persist.Account) error {

	_, err := a.accountStorage.upsert(pCtx, bson.M{
		"address": strings.ToLower(pAddress.String()),
	}, pUpsert)
	if err != nil {
		return err
	}

	return nil
}

// GetByAddress returns an account by a given address
func (a *AccountMongoRepository) GetByAddress(pCtx context.Context, pAddress persist.Address) (*persist.Account, error) {

	result := []*persist.Account{}
	err := a.accountStorage.find(pCtx, bson.M{"address": strings.ToLower(pAddress.String())}, &result)
	if err != nil {
		return nil, err
	}

	if len(result) < 1 {
		return nil, persist.ErrAccountNotFoundByAddress{Address: pAddress}
	}

	if len(result) > 1 {
		logrus.Errorf("found more than one account for address: %s", pAddress)
	}

	return result[0], nil
}
