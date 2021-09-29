package persist

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/segmentio/ksuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// redisDB represents the database number to use for the redis client
type redisDB int

const (
	// CollectionsUnassignedRDB is a throttled cache for expensive queries finding unassigned NFTs
	CollectionsUnassignedRDB redisDB = iota
	// OpenseaAssetsRDB is a throttled cache for expensive queries finding Opensea NFTs
	OpenseaAssetsRDB
)

var (
	collectionUnassignedTTL time.Duration = time.Minute * 1
	openseaAssetsTTL        time.Duration = time.Minute * 5
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
		Password: runtime.Config.RedisPassword,
		DB:       int(rdb),
	})
	m.rdbClient = client
	return m
}

// Insert inserts a document into the mongo database while filling out the fields id, creation time, and last updated
func (m *storage) insert(ctx context.Context, insert interface{}, opts ...*options.InsertOneOptions) (DBID, error) {

	now := primitive.NewDateTimeFromTime(time.Now())
	asMap, err := structToBsonMap(insert)
	if err != nil {
		return "", err
	}
	asMap["created_at"] = now
	asMap["last_updated"] = now
	asMap["_id"] = generateID(asMap)

	res, err := m.collection.InsertOne(ctx, asMap, opts...)
	if err != nil {
		return "", err
	}

	return DBID(res.InsertedID.(string)), nil
}

// InsertMany inserts many documents into a mongo database while filling out the fields id, creation time, and last updated for each
func (m *storage) insertMany(ctx context.Context, insert []interface{}, opts ...*options.InsertManyOptions) ([]DBID, error) {

	mapsToInsert := make([]interface{}, len(insert))
	for i, k := range insert {
		now := primitive.NewDateTimeFromTime(time.Now())
		asMap, err := structToBsonMap(k)
		if err != nil {
			return nil, err
		}
		asMap["created_at"] = now
		asMap["last_updated"] = now
		asMap["_id"] = generateID(asMap)
		mapsToInsert[i] = asMap
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
func (m *storage) update(ctx context.Context, query bson.M, update interface{}, opts ...*options.UpdateOptions) error {
	now := primitive.NewDateTimeFromTime(time.Now())

	asMap, err := structToBsonMap(update)
	if err != nil {
		return err
	}
	asMap["last_updated"] = now

	result, err := m.collection.UpdateMany(ctx, query, bson.D{{Key: "$set", Value: asMap}}, opts...)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return &DocumentNotFoundError{}
	}

	return nil
}

// push pushes items into an array field for a given queried document(s)
// value must be an array
func (m *storage) push(ctx context.Context, query bson.M, field string, value interface{}) error {

	push := bson.E{Key: "$push", Value: bson.M{field: bson.M{"$each": value}}}
	lastUpdated := bson.E{Key: "$set", Value: bson.M{"last_updated": primitive.NewDateTimeFromTime(time.Now())}}
	up := bson.D{push, lastUpdated}

	result, err := m.collection.UpdateMany(ctx, query, up)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return &DocumentNotFoundError{}
	}

	return nil
}

// pullAll pulls all items from an array field for a given queried document(s)
// value must be an array
func (m *storage) pullAll(ctx context.Context, query bson.M, field string, value interface{}) error {

	pull := bson.E{Key: "$pullAll", Value: bson.M{field: value}}
	lastUpdated := bson.E{Key: "$set", Value: bson.M{"last_updated": primitive.NewDateTimeFromTime(time.Now())}}
	up := bson.D{pull, lastUpdated}

	result, err := m.collection.UpdateMany(ctx, query, up)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return &DocumentNotFoundError{}
	}

	return nil
}

// pull puls items from an array field for a given queried document(s)
func (m *storage) pull(ctx context.Context, query bson.M, field string, value bson.M) error {

	pull := bson.E{Key: "$pull", Value: bson.M{field: value}}
	lastUpdated := bson.E{Key: "$set", Value: bson.M{"last_updated": primitive.NewDateTimeFromTime(time.Now())}}
	up := bson.D{pull, lastUpdated}

	result, err := m.collection.UpdateMany(ctx, query, up)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return &DocumentNotFoundError{}
	}

	return nil
}

// Upsert upserts a document in the mongo database while filling out the fields id, creation time, and last updated
func (m *storage) upsert(ctx context.Context, query bson.M, upsert interface{}, opts ...*options.UpdateOptions) (DBID, error) {
	returnID := DBID("")
	opts = append(opts, &options.UpdateOptions{Upsert: boolin(true)})
	now := primitive.NewDateTimeFromTime(time.Now())
	asMap, err := structToBsonMap(upsert)
	if err != nil {
		return "", err
	}
	asMap["last_updated"] = now
	if _, ok := asMap["created_at"]; !ok {
		asMap["created_at"] = now
	}

	if id, ok := asMap["_id"]; ok && id != "" {
		returnID = id.(DBID)
	}

	delete(asMap, "_id")
	for k := range query {
		delete(asMap, k)
	}

	res, err := m.collection.UpdateOne(ctx, query, bson.M{"$setOnInsert": bson.M{"_id": generateID(asMap)}, "$set": asMap}, opts...)
	if err != nil {
		return "", err
	}

	if it, ok := res.UpsertedID.(string); ok {
		returnID = DBID(it)
	}

	return returnID, nil
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

// cacheDelete deletes a value from the redis cache
func (m *storage) cacheDelete(ctx context.Context, key string) error {
	if m.rdbClient == nil {
		return errors.New("no redis client attached to storage instance")
	}
	return m.rdbClient.Del(ctx, key).Err()
}

// cacheClose closes the redis client
func (m *storage) cacheClose() error {
	if m.rdbClient == nil {
		return errors.New("no redis client attached to storage instance")
	}
	return m.rdbClient.Close()
}

func generateID(it interface{}) DBID {
	id, err := ksuid.NewRandom()
	if err != nil {
		panic(err)
	}
	return DBID(id.String())
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
				case "only_get":
					continue
				}

			}
			if tag == "-" {
				continue
			}
			if field.CanInterface() {
				bsonMap[spl[0]] = field.Interface()
			}
		}
	}
	return bsonMap, nil
}

// a function that returns true if the value is a zero value or nil
func isValueEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}

// function that returns the DBIDs that are in one array but not in the other
func idDiff(a, b []DBID) []DBID {
	m := make(map[DBID]bool)
	for _, item := range b {
		m[item] = true
	}
	var diff []DBID
	for _, item := range a {
		if _, ok := m[item]; !ok {
			diff = append(diff, item)
		}
	}
	return diff
}
