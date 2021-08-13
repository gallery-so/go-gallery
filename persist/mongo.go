package persist

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/copy"
	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DBID represents a mongo database ID
type DBID string

// MongoStorage represents the currently accessed collection and the version of the "schema"
type MongoStorage struct {
	version    int64
	collection *mongo.Collection
}

// NewMongoStorage returns a new MongoStorage instance with a pointer to a collection of the specified name
// and the specified version
func NewMongoStorage(version int64, collName string, runtime *runtime.Runtime) *MongoStorage {
	coll := runtime.DB.MongoDB.Collection(collName)
	return &MongoStorage{version: version, collection: coll}
}

// Insert inserts a document into the mongo database while filling out the fields id, creation time, and last updated
func (m *MongoStorage) Insert(ctx context.Context, insert interface{}, opts ...*options.InsertOneOptions) (DBID, error) {

	now := float64(time.Now().UnixNano()) / 1000000000.0
	asMap, err := structToBsonMap(insert)
	if err != nil {
		return "", err
	}
	asMap["_id"] = generateID(now)
	asMap["created_at"] = now
	asMap["last_updated"] = now

	res, err := m.collection.InsertOne(ctx, asMap, opts...)
	if err != nil {
		return "", err
	}

	return DBID(res.InsertedID.(string)), nil
}

// InsertMany inserts many documents into a mongo database while filling out the fields id, creation time, and last updated for each
func (m *MongoStorage) InsertMany(ctx context.Context, insert []interface{}, opts ...*options.InsertManyOptions) ([]DBID, error) {

	mapsToInsert := make([]interface{}, len(insert))
	for _, k := range insert {
		now := float64(time.Now().UnixNano()) / 1000000000.0
		asMap, err := structToBsonMap(k)
		if err != nil {
			return nil, err
		}
		asMap["_id"] = generateID(now)
		asMap["created_at"] = now
		asMap["last_updated"] = now
		mapsToInsert = append(mapsToInsert, asMap)
	}

	res, err := m.collection.InsertMany(ctx, mapsToInsert, opts...)
	if err != nil {
		return nil, err
	}

	ids := make([]DBID, len(res.InsertedIDs))

	for i, v := range res.InsertedIDs {
		if id, ok := v.(string); ok {
			ids[i] = DBID(id)
		}
	}
	return ids, nil
}

// Update updates a document in the mongo database while filling out the field LastUpdated
func (m *MongoStorage) Update(ctx context.Context, query bson.M, update interface{}, opts ...*options.UpdateOptions) error {
	now := float64(time.Now().UnixNano()) / 1000000000.0

	asMap, err := structToBsonMap(update)
	if err != nil {
		return err
	}
	asMap["last_updated"] = now

	result, err := m.collection.UpdateOne(ctx, query, bson.D{{Key: "$set", Value: asMap}}, opts...)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		// TODO this should return a 404 or 204
		return errors.New(copy.CouldNotFindDocument)
	}

	return nil
}

// Push pushes items into an array field for a given queried document
func (m *MongoStorage) Push(ctx context.Context, query bson.M, field string, value ...interface{}) error {

	result, err := m.collection.UpdateOne(ctx, query, bson.D{{Key: "$push", Value: bson.M{field: bson.M{"$each": value}}}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		// TODO this should return a 404 or 204
		return errors.New(copy.CouldNotFindDocument)
	}

	return nil
}

// Upsert upserts a document in the mongo database while filling out the fields id, creation time, and last updated
func (m *MongoStorage) Upsert(ctx context.Context, query bson.M, upsert interface{}, opts ...*options.UpdateOptions) error {
	weWantToUpsertHere := true
	opts = append(opts, &options.UpdateOptions{Upsert: &weWantToUpsertHere})
	now := float64(time.Now().UnixNano()) / 1000000000.0
	asMap, err := structToBsonMap(upsert)
	if err != nil {
		return err
	}
	asMap["last_updated"] = now

	result, err := m.collection.UpdateOne(ctx, query, bson.M{"$setOnInsert": bson.M{"_id": generateID(now), "created_at": now}, "$set": asMap}, opts...)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return errors.New(copy.CouldNotFindDocument)
	}

	return nil
}

// Find finds documents in the mongo database which is not deleted
// result must be a slice of pointers to the struct of the type expected to be decoded from mongo
func (m *MongoStorage) Find(ctx context.Context, filter bson.M, result interface{}, opts ...*options.FindOptions) error {

	filter["deleted"] = false

	cur, err := m.collection.Find(ctx, filter, opts...)
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	return cur.All(ctx, result)

}

// Aggregate performs an aggregation operation on the mongo database
// result must be a pointer to a slice of structs, map[string]interface{}, or bson structs
func (m *MongoStorage) Aggregate(ctx context.Context, agg mongo.Pipeline, result interface{}, opts ...*options.AggregateOptions) error {

	cur, err := m.collection.Aggregate(ctx, agg, opts...)
	if err != nil {
		return err
	}

	return cur.All(ctx, result)

}

// Count counts the number of documents in the mongo database which is not deleted
// result must be a pointer to a slice of structs, map[string]interface{}, or bson structs
func (m *MongoStorage) Count(ctx context.Context, filter bson.M, opts ...*options.CountOptions) (int64, error) {
	filter["deleted"] = false
	return m.collection.CountDocuments(ctx, filter, opts...)
}

// CreateIndex creates a new index in the mongo database
func (m *MongoStorage) CreateIndex(ctx context.Context, index mongo.IndexModel, opts ...*options.CreateIndexesOptions) (string, error) {
	return m.collection.Indexes().CreateOne(ctx, index, opts...)
}

func generateID(creationTime float64) DBID {
	h := md5.New()
	h.Write([]byte(fmt.Sprint(creationTime)))
	sum := h.Sum(nil)
	hexStr := hex.EncodeToString(sum)
	return DBID(hexStr)
}

// function that returns the pointer to the bool passed in
func boolin(b bool) *bool {
	return &b
}

func structToBsonMap(v interface{}) (bson.M, error) {
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%v is not a struct, is of type %T", v, v)
	}
	bsonMap := bson.M{}
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		tag, ok := val.Type().Field(i).Tag.Lookup("bson")
		if ok {
			spl := strings.Split(tag, ",")
			if len(spl) > 1 {
				switch spl[1] {
				case "omitempty":
					if isValueEmpty(field) {
						continue
					}
				}
			}
			if field.CanInterface() {
				bsonMap[spl[0]] = field.Interface()
			}
		}
	}
	return bsonMap, nil
}

func isValueEmpty(field reflect.Value) bool {
	if field.IsZero() {
		return true
	}
	return field.IsNil()
}
