package persist

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type DbId string

type MongoStorage struct {
	version    int64
	collection *mongo.Collection
}

func NewMongoStorage(version int64, collName string, runtime *runtime.Runtime) *MongoStorage {
	coll := runtime.DB.MongoDB.Collection(collName)
	return &MongoStorage{version: version, collection: coll}
}

// insert must be a pointer to a struct
func (m *MongoStorage) Insert(ctx context.Context, insert interface{}, opts ...*options.InsertOneOptions) (DbId, error) {

	elem := reflect.TypeOf(insert).Elem()
	val := reflect.ValueOf(insert).Elem()
	now := float64(time.Now().UnixNano()) / 1000000000.0
	if _, ok := elem.FieldByName("IDstr"); ok {
		idField := val.FieldByName("IDstr")
		if !idField.CanSet() {
			// panic because this literally cannot happen in prod
			panic("unable to set id field on struct")
		}
		if _, ok = elem.FieldByName("CreationTimeF"); ok {
			val.FieldByName("CreationTimeF").SetFloat(now)
		} else {
			// panic because this literally cannot happen in prod
			panic("creation time field required for id-able structs")
		}
		idField.Set(reflect.ValueOf(generateId(now)))
	}

	if _, ok := elem.FieldByName("LastUpdatedF"); ok {
		f := val.FieldByName("LastUpdatedF")
		if f.CanSet() {
			f.SetFloat(now)
		}
	}

	res, err := m.collection.InsertOne(ctx, insert, opts...)
	if err != nil {
		return "", err
	}

	return DbId(res.InsertedID.(string)), nil
}

// insert must be a slice of pointers to a struct
func (m *MongoStorage) InsertMany(ctx context.Context, insert []interface{}, opts ...*options.InsertManyOptions) ([]DbId, error) {

	for _, k := range insert {
		elem := reflect.TypeOf(k).Elem()
		val := reflect.ValueOf(k).Elem()
		now := float64(time.Now().UnixNano()) / 1000000000.0
		if _, ok := elem.FieldByName("IDstr"); ok {
			idField := val.FieldByName("IDstr")
			if !idField.CanSet() {
				// panic because this literally cannot happen in prod
				panic("unable to set id field on struct")
			}
			if _, ok = elem.FieldByName("CreationTimeF"); ok {
				val.FieldByName("CreationTimeF").SetFloat(now)
			} else {
				// panic because this literally cannot happen in prod
				panic("creation time field required for id-able structs")
			}
			idField.Set(reflect.ValueOf(generateId(now)))
		}

		if _, ok := elem.FieldByName("LastUpdatedF"); ok {
			f := val.FieldByName("LastUpdatedF")
			if f.CanSet() {
				f.SetFloat(now)
			}
		}
	}

	res, err := m.collection.InsertMany(ctx, insert, opts...)
	if err != nil {
		return nil, err
	}

	ids := make([]DbId, len(res.InsertedIDs))

	for i, v := range res.InsertedIDs {
		if id, ok := v.(string); ok {
			ids[i] = DbId(id)
		}
	}
	return ids, nil
}

// update must be a pointer to a struct
func (m *MongoStorage) Update(ctx context.Context, query bson.M, update interface{}, opts ...*options.UpdateOptions) error {

	elem := reflect.TypeOf(update).Elem()
	val := reflect.ValueOf(update).Elem()
	if _, ok := elem.FieldByName("LastUpdatedF"); ok {
		f := val.FieldByName("LastUpdatedF")
		if f.CanSet() {
			now := float64(time.Now().UnixNano()) / 1000000000.0
			f.SetFloat(now)
		}
	}
	result, err := m.collection.UpdateOne(ctx, query, bson.D{{Key: "$set", Value: update}}, opts...)
	if err != nil {
		return err
	}
	if result.ModifiedCount == 0 || result.MatchedCount == 0 {
		return errors.New("could not find document to update")
	}

	return nil
}

// result must be a slice of pointers to the struct of the type expected to be decoded from mongo
func (m *MongoStorage) Find(ctx context.Context, filter bson.M, result interface{}, opts ...*options.FindOptions) error {

	filter["deleted"] = false

	cur, err := m.collection.Find(ctx, filter, opts...)
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	if err := cur.All(ctx, result); err != nil {
		return errors.New("could not decode cursor")
	}
	return nil
}

// result must be a pointer to a slice of structs, map[string]interface{}, or bson structs
func (m *MongoStorage) Aggregate(ctx context.Context, agg mongo.Pipeline, result interface{}, opts ...*options.AggregateOptions) error {

	cur, err := m.collection.Aggregate(ctx, agg, opts...)
	if err != nil {
		return err
	}

	return cur.All(ctx, result)

}

// result must be a pointer to a slice of structs, map[string]interface{}, or bson structs
func (m *MongoStorage) Count(ctx context.Context, filter bson.M, opts ...*options.CountOptions) (int64, error) {
	return m.collection.CountDocuments(ctx, filter, opts...)
}

// CREATE_ID
func generateId(creationTime float64) DbId {
	h := md5.New()
	h.Write([]byte(fmt.Sprint(creationTime)))
	sum := h.Sum(nil)
	hexStr := hex.EncodeToString(sum)
	return DbId(hexStr)
}
