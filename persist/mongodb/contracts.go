package mongodb

import (
	"context"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const contractsCollName = "contracts"

// ContractMongoRepository is a repository for storing authentication nonces in a MongoDB database
type ContractMongoRepository struct {
	mp *storage
}

// NewContractMongoRepository returns a new instance of a login attempt repository
func NewContractMongoRepository(mgoClient *mongo.Client) *ContractMongoRepository {
	return &ContractMongoRepository{
		mp: newStorage(mgoClient, 0, galleryDBName, contractsCollName),
	}
}

// UpsertByAddress upserts an contract by a given address
// pUpdate represents a struct with bson tags to specify which fields to update
func (c *ContractMongoRepository) UpsertByAddress(pCtx context.Context, pAddress string, pUpsert *persist.Contract) error {

	_, err := c.mp.upsert(pCtx, bson.M{
		"address": strings.ToLower(pAddress),
	}, pUpsert)
	if err != nil {
		return err
	}

	return nil
}

// GetByAddress returns an contract by a given address
func (c *ContractMongoRepository) GetByAddress(pCtx context.Context, pAddress string) (*persist.Contract, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Contract{}
	err := c.mp.find(pCtx, bson.M{"address": strings.ToLower(pAddress)}, &result, opts)

	if err != nil {
		return nil, err
	}

	if len(result) < 1 {
		return nil, persist.ErrContractNotFoundByAddress{Address: pAddress}
	}

	if len(result) > 1 {
		logrus.Errorf("found more than one contract for address: %s", pAddress)
	}

	return result[0], nil
}
