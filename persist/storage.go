package persist

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/mikeydub/go-gallery/copy"
	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// RedisDB represents the database number to use for the redis client
type redisDB int

const (
	// CollectionsUnassignedRDB is a throttled cache for expensive queries finding unassigned NFTs
	CollectionsUnassignedRDB redisDB = iota
	// OpenseaGetRDB is a throttled cache for expensive queries finding Opensea NFTs
	OpenseaGetRDB
)

var (
	collectionUnassignedTTL time.Duration = time.Minute * 1
	opeseaGetTTL            time.Duration = time.Minute * 1
)

// DBID represents a mongo database ID
type DBID string

// storage represents the currently accessed collection and the version of the "schema"
type storage struct {
	version    int64
	collection *mongo.Collection
	rdbClient  *redis.Client
}

// newStorage returns a new MongoStorage instance with a pointer to a collection of the specified name
// and the specified version
func newStorage(version int64, collName string, runtime *runtime.Runtime) *storage {
	coll := runtime.DB.MongoDB.Collection(collName)

	return &storage{version: version, collection: coll}
}

// withRedis attaches a redis client to the Storage instance
func (m *storage) withRedis(rdb redisDB, runtime *runtime.Runtime) *storage {
	client := redis.NewClient(&redis.Options{
		Addr:     runtime.Config.RedisURL,
		Password: runtime.Config.RedisPassword, // no password set
		DB:       int(rdb),                     // use default DB
	})
	m.rdbClient = client
	return m
}

// insert inserts a document into the mongo database while filling out the fields id, creation time, and last updated
// insert must be a pointer to a struct
func (m *storage) insert(ctx context.Context, insert interface{}, opts ...*options.InsertOneOptions) (DBID, error) {

	elem := reflect.TypeOf(insert).Elem()
	val := reflect.ValueOf(insert).Elem()
	now := float64(time.Now().UnixNano()) / 1000000000.0

	if _, ok := elem.FieldByName("ID"); ok {
		idField := val.FieldByName("ID")
		if !idField.CanSet() {
			// panic because this literally cannot happen in prod
			panic("unable to set id field on struct")
		}
		if _, ok = elem.FieldByName("CreationTime"); ok {
			val.FieldByName("CreationTime").SetFloat(now)
		} else {
			// panic because this literally cannot happen in prod
			panic("creation time field required for id-able structs")
		}
		idField.Set(reflect.ValueOf(generateID(now)))
	}

	if _, ok := elem.FieldByName("LastUpdated"); ok {
		f := val.FieldByName("LastUpdated")
		if f.CanSet() {
			f.SetFloat(now)
		}
	}

	res, err := m.collection.InsertOne(ctx, insert, opts...)
	if err != nil {
		return "", err
	}

	return DBID(res.InsertedID.(string)), nil
}

// InsertMany inserts many documents into a mongo database while filling out the fields id, creation time, and last updated for each
// insert must be a slice of pointers to a struct
func (m *storage) InsertMany(ctx context.Context, insert []interface{}, opts ...*options.InsertManyOptions) ([]DBID, error) {

	for _, k := range insert {
		elem := reflect.TypeOf(k).Elem()
		val := reflect.ValueOf(k).Elem()
		now := float64(time.Now().UnixNano()) / 1000000000.0
		if _, ok := elem.FieldByName("ID"); ok {
			idField := val.FieldByName("ID")
			if !idField.CanSet() {
				// panic because this literally cannot happen in prod
				panic("unable to set id field on struct")
			}
			if _, ok = elem.FieldByName("CreationTime"); ok {
				val.FieldByName("CreationTime").SetFloat(now)
			} else {
				// panic because this literally cannot happen in prod
				panic("creation time field required for id-able structs")
			}
			idField.Set(reflect.ValueOf(generateID(now)))
		}

		if _, ok := elem.FieldByName("LastUpdated"); ok {
			f := val.FieldByName("LastUpdated")
			if f.CanSet() {
				f.SetFloat(now)
			}
		}
	}

	res, err := m.collection.InsertMany(ctx, insert, opts...)
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

// update updates a document in the mongo database while filling out the field LastUpdated
// update must be a pointer to a struct
func (m *storage) update(ctx context.Context, query bson.M, update interface{}, opts ...*options.UpdateOptions) error {
	elem := reflect.TypeOf(update).Elem()
	val := reflect.ValueOf(update).Elem()
	if _, ok := elem.FieldByName("LastUpdated"); ok {
		f := val.FieldByName("LastUpdated")
		if f.CanSet() {
			now := float64(time.Now().UnixNano()) / 1000000000.0
			f.SetFloat(now)
		}
	}

	result, err := m.collection.UpdateOne(ctx, query, bson.D{{Key: "$set", Value: update}}, opts...)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		// TODO this should return a 404 or 204
		return errors.New(copy.CouldNotFindDocument)
	}

	return nil
}

// upsert upserts a document in the mongo database while filling out the fields id, creation time, and last updated
// upsert must be a pointer to a struct, will not fill reflectively fill insert fields such as id or creation time
func (m *storage) upsert(ctx context.Context, query bson.M, upsert interface{}, opts ...*options.UpdateOptions) error {
	weWantToUpsertHere := true
	opts = append(opts, &options.UpdateOptions{Upsert: &weWantToUpsertHere})
	elem := reflect.TypeOf(upsert).Elem()
	val := reflect.ValueOf(upsert).Elem()
	now := float64(time.Now().UnixNano()) / 1000000000.0
	if _, ok := elem.FieldByName("LastUpdated"); ok {
		f := val.FieldByName("LastUpdated")
		if f.CanSet() {
			f.SetFloat(now)
		}
	}

	result, err := m.collection.UpdateOne(ctx, query, bson.M{"$setOnInsert": bson.M{"_id": generateID(now), "created_at": now}, "$set": upsert}, opts...)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return errors.New(copy.CouldNotFindDocument)
	}

	return nil
}

// find finds documents in the mongo database which is not deleted
// result must be a slice of pointers to the struct of the type expected to be decoded from mongo
func (m *storage) find(ctx context.Context, filter bson.M, result interface{}, opts ...*options.FindOptions) error {

	filter["deleted"] = false

	cur, err := m.collection.Find(ctx, filter, opts...)
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	return cur.All(ctx, result)

}

// aggregate performs an aggregation operation on the mongo database
// result must be a pointer to a slice of structs, map[string]interface{}, or bson structs
func (m *storage) aggregate(ctx context.Context, agg mongo.Pipeline, result interface{}, opts ...*options.AggregateOptions) error {

	cur, err := m.collection.Aggregate(ctx, agg, opts...)
	if err != nil {
		return err
	}

	return cur.All(ctx, result)

}

// count counts the number of documents in the mongo database which is not deleted
// result must be a pointer to a slice of structs, map[string]interface{}, or bson structs
func (m *storage) count(ctx context.Context, filter bson.M, opts ...*options.CountOptions) (int64, error) {
	filter["deleted"] = false
	return m.collection.CountDocuments(ctx, filter, opts...)
}

// createIndex creates a new index in the mongo database
func (m *storage) createIndex(ctx context.Context, index mongo.IndexModel, opts ...*options.CreateIndexesOptions) (string, error) {
	return m.collection.Indexes().CreateOne(ctx, index, opts...)
}

// cacheSet sets a value in the redis cache
func (m *storage) cacheSet(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	if m.rdbClient == nil {
		return errors.New("no redis client attached to storage instance")
	}
	return m.rdbClient.Set(ctx, key, value, expiration).Err()
}

// cacheSetKeepTTL sets a value in the redis cache without resetting TTL
func (m *storage) cacheSetKeepTTL(ctx context.Context, key string, value interface{}) error {
	if m.rdbClient == nil {
		return errors.New("no redis client attached to storage instance")
	}
	return m.rdbClient.Set(ctx, key, value, redis.KeepTTL).Err()
}

// cacheGet gets a value from the redis cache
func (m *storage) cacheGet(ctx context.Context, key string) (string, error) {
	if m.rdbClient == nil {
		return "", errors.New("no redis client attached to storage instance")
	}
	return m.rdbClient.Get(ctx, key).Result()
}

// cacheClose closes the redis client
func (m *storage) cacheClose() error {
	if m.rdbClient == nil {
		return errors.New("no redis client attached to storage instance")
	}
	return m.rdbClient.Close()
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
