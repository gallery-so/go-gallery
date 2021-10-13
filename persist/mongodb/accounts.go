package mongodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/persist"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const accountCollName = "accounts"

// AccountMongoRepository is a repository for storing authentication nonces in a MongoDB database
type AccountMongoRepository struct {
	mp *storage
}

type errAccountNotFoundByAddress struct {
	address string
}

// NewAccountMongoRepository returns a new instance of a login attempt repository
func NewAccountMongoRepository(mgoClient *mongo.Client) *AccountMongoRepository {
	return &AccountMongoRepository{
		mp: newStorage(mgoClient, 0, galleryDBName, accountCollName),
	}
}

// UpsertByAddress upserts an account by a given address
// pUpdate represents a struct with bson tags to specify which fields to update
func (a *AccountMongoRepository) UpsertByAddress(pCtx context.Context, pAddress string, pUpsert *persist.Account) error {

	_, err := a.mp.upsert(pCtx, bson.M{
		"address": strings.ToLower(pAddress),
	}, pUpsert)
	if err != nil {
		return err
	}

	return nil
}

// GetByAddress returns an account by a given address
func (a *AccountMongoRepository) GetByAddress(pCtx context.Context, pAddress string) (*persist.Account, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Account{}
	err := a.mp.find(pCtx, bson.M{"address": strings.ToLower(pAddress)}, &result, opts)
	if err != nil {
		return nil, err
	}

	if len(result) != 1 {
		return nil, errAccountNotFoundByAddress{address: pAddress}
	}

	return result[0], nil
}

func (e errAccountNotFoundByAddress) Error() string {
	return fmt.Sprintf("account not found by address: %s", e.address)
}
