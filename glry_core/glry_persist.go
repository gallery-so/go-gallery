package glry_core

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type GLRYdbId string

type GLRYmongoPersister interface {
	Insert(context.Context, interface{}, ...*options.InsertOneOptions) error
	Update(context.Context, bson.M, interface{}, ...*options.UpdateOptions) error
	Find(context.Context, bson.M, interface{}, ...*options.FindOptions) error
}

type GLRYmongoPersisterImpl struct {
	Version    int64
	Collection *mongo.Collection
}

func NewMongoPersister(version int64, collName string, runtime *Runtime) GLRYmongoPersister {
	coll := runtime.DB.MongoDB.Collection(collName)
	return &GLRYmongoPersisterImpl{Version: version, Collection: coll}
}

// insert must be a pointer to a struct
func (m *GLRYmongoPersisterImpl) Insert(ctx context.Context, insert interface{}, opts ...*options.InsertOneOptions) error {

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

	_, err := m.Collection.InsertOne(ctx, insert, opts...)
	if err != nil {
		return err
	}

	return nil
}

// update must be a pointer to a struct
func (m *GLRYmongoPersisterImpl) Update(ctx context.Context, query bson.M, update interface{}, opts ...*options.UpdateOptions) error {

	elem := reflect.TypeOf(update).Elem()
	val := reflect.ValueOf(update).Elem()
	if _, ok := elem.FieldByName("LastUpdatedF"); ok {
		f := val.FieldByName("LastUpdatedF")
		if f.CanSet() {
			now := float64(time.Now().UnixNano()) / 1000000000.0
			f.SetFloat(now)
		}
	}
	result, err := m.Collection.UpdateOne(ctx, query, bson.D{{"$set", update}}, opts...)
	if err != nil {
		return err
	}
	if result.ModifiedCount == 0 || result.MatchedCount == 0 {
		return errors.New("could not find document to update")
	}

	return nil
}

// result must be a slice of pointers to the struct of the type expected to be decoded from mongo
func (m *GLRYmongoPersisterImpl) Find(ctx context.Context, filter bson.M, result interface{}, opts ...*options.FindOptions) error {

	filter["deleted"] = false

	cur, err := m.Collection.Find(ctx, filter, opts...)
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	if err := cur.All(ctx, result); err != nil {
		return errors.New("could not decode cursor")
	}
	return nil
}

// CREATE_ID
func generateId(creationTime float64) GLRYdbId {
	h := md5.New()
	h.Write([]byte(fmt.Sprint(creationTime)))
	sum := h.Sum(nil)
	hexStr := hex.EncodeToString(sum)
	return GLRYdbId(hexStr)
}
