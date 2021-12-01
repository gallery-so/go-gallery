package mongodb

import (
	"context"
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
func (c *ContractMongoRepository) UpsertByAddress(pCtx context.Context, pAddress persist.Address, pUpsert *persist.Contract) error {

	_, err := c.mp.upsert(pCtx, bson.M{
		"address": pAddress,
	}, pUpsert)
	if err != nil {
		return err
	}

	return nil
}

// BulkUpsert upserts many contracts by their address field
func (c *ContractMongoRepository) BulkUpsert(pCtx context.Context, contracts []*persist.Contract) error {

	upserts := make([]updateModel, len(contracts))
	for i, contract := range contracts {

		setDocs := make(bson.D, 0, 2)
		query := bson.M{
			"address": contract.Address,
		}
		asBSON, err := bson.MarshalWithRegistry(CustomRegistry, contract)
		if err != nil {
			return err
		}

		asMap := bson.M{}
		err = bson.UnmarshalWithRegistry(CustomRegistry, asBSON, &asMap)
		if err != nil {
			return err
		}
		delete(asMap, "_id")

		for k := range query {
			delete(asMap, k)
		}
		now := time.Now()
		asMap["last_updated"] = now

		setDocs = append(setDocs, bson.E{Key: "$set", Value: asMap})

		insertDoc := bson.E{Key: "$setOnInsert", Value: bson.M{"_id": persist.GenerateID(), "created_at": now}}
		setDocs = append(setDocs, insertDoc)

		upserts[i] = updateModel{
			query:   query,
			setDocs: setDocs,
		}
	}
	err := c.mp.bulkUpdate(pCtx, upserts, true)
	if err != nil {
		return err
	}

	return nil
}

// GetByAddress returns an contract by a given address
func (c *ContractMongoRepository) GetByAddress(pCtx context.Context, pAddress persist.Address) (*persist.Contract, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Contract{}
	err := c.mp.find(pCtx, bson.M{"address": pAddress}, &result, opts)

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
