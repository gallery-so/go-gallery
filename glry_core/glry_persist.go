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

type GLRYpersister interface {
	// TODO insert many, update many
	Insert(context.Context, interface{}, interface{}) error
	Update(context.Context, string, interface{}, interface{}) error
	Find(context.Context, interface{}, interface{}, []interface{}) error
}

type GLRYmongoPersister struct {
	Version    int64
	Collection *mongo.Collection
}

func NewMongoPersister(version int64, collName string, runtime Runtime) GLRYpersister {
	collection := runtime.DB.MongoDB.Collection(collName)
	return GLRYmongoPersister{Version: version, Collection: collection}
}

// i must be a pointer to a struct
func (m GLRYmongoPersister) Insert(ctx context.Context, insert interface{}, opts interface{}) error {

	insertOpts, ok := opts.(*options.InsertOneOptions)

	if !ok {
		return errors.New("opts must be of type *options.InsertOneOptions")
	}
	elem := reflect.TypeOf(insert).Elem()
	val := reflect.ValueOf(insert).Elem()
	if _, ok := elem.FieldByName("IDstr"); ok {
		fieldsForId := []interface{}{}
		idField := val.FieldByName("IDstr")
		if !idField.CanSet() {
			return errors.New("must be able to set id field")
		}
		for i := 0; i < elem.NumField(); i++ {
			f := elem.Field(i)
			t := f.Tag
			if v, ok := t.Lookup("fill"); ok {
				switch v {
				case "creation_time":
					now := float64(time.Now().UnixNano()) / 1000000000.0
					if val.Field(i).CanSet() {
						val.Field(i).SetFloat(now)
					}
					fieldsForId = append(fieldsForId, now)
				case "version":
					if val.Field(i).CanSet() {
						val.Field(i).SetInt(m.Version)
					}
					fieldsForId = append(fieldsForId, m.Version)
				default:
					add := val.Field(i)
					fieldsForId = append(fieldsForId, add)
				}
			}
		}
		idField.Set(reflect.ValueOf(CreateId(fieldsForId...)))
	}

	if _, ok := elem.FieldByName("LastUpdatedF"); ok {
		f := val.FieldByName("LastUpdatedF")
		if f.CanSet() {
			now := float64(time.Now().UnixNano()) / 1000000000.0
			f.SetFloat(now)
		}
	}

	_, err := m.Collection.InsertOne(ctx, insert, insertOpts)
	if err != nil {
		return err
	}

	return nil
}

// i must be a pointer to a struct
func (m GLRYmongoPersister) Update(ctx context.Context, id string, update interface{}, opts interface{}) error {
	updateOpts, ok := opts.(*options.UpdateOptions)

	if !ok {
		return errors.New("opts must be of type *options.UpdateOptions")
	}
	elem := reflect.TypeOf(update).Elem()
	val := reflect.ValueOf(update).Elem()
	if _, ok := elem.FieldByName("LastUpdatedF"); ok {
		f := val.FieldByName("LastUpdatedF")
		if f.CanSet() {
			now := float64(time.Now().UnixNano()) / 1000000000.0
			f.SetFloat(now)
		}
	}

	result, err := m.Collection.UpdateByID(ctx, id, update, updateOpts)
	if err != nil {
		return err
	}
	if result.ModifiedCount == 0 || result.MatchedCount == 0 {
		return errors.New("could not find document to update")
	}

	return nil
}

// result must be a slice of pointers to the struct of the type expected to be decoded from mongo
func (m GLRYmongoPersister) Find(ctx context.Context, filter interface{}, opts interface{}, result []interface{}) error {
	fil, ok := filter.(bson.M)
	if !ok {
		return errors.New("filter must be of type bson.M")
	}
	findOpts, ok := opts.(*options.FindOptions)
	if !ok {
		return errors.New("opts must be of type *options.FindOptions")
	}
	cur, err := m.Collection.Find(ctx, fil, findOpts)
	if err != nil {
		return err
	}
	return cur.All(ctx, result)
}

// CREATE_ID
func CreateId(fields ...interface{}) GLRYdbId {
	h := md5.New()
	for _, field := range fields {
		h.Write([]byte(fmt.Sprint(field)))
	}
	sum := h.Sum(nil)
	hexStr := hex.EncodeToString(sum)
	return GLRYdbId(hexStr)
}
