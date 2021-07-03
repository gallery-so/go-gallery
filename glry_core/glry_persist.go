package glry_core

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"reflect"
	"time"

	"github.com/gloflow/gloflow/go/gf_core"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type GLRYdbId string

type GLRYmongoPersister interface {
	// TODO insert many, update many
	Insert(context.Context, interface{}, map[string]interface{}) *gf_core.Gf_error
	Update(context.Context, bson.M, interface{}, map[string]interface{}) *gf_core.Gf_error
	Find(context.Context, bson.M, []interface{}, *options.FindOptions, map[string]interface{}) *gf_core.Gf_error
}

type GLRYmongoPersisterImpl struct {
	Version int64
	// Collection  *mongo.Collection
	CollNameStr string
	Runtime     Runtime
}

func NewMongoPersister(version int64, collName string, runtime Runtime) GLRYmongoPersister {

	return GLRYmongoPersisterImpl{Version: version, CollNameStr: collName, Runtime: runtime}
}

// i must be a pointer to a struct
func (m GLRYmongoPersisterImpl) Insert(ctx context.Context, insert interface{}, meta map[string]interface{}) *gf_core.Gf_error {

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

	return gf_core.Mongo__insert(insert, m.CollNameStr, meta, ctx, m.Runtime.RuntimeSys)

	// _, err := m.Collection.InsertOne(ctx, insert, opts...)
	// if err != nil {
	// 	return err
	// }

	// return nil
}

// update must be a pointer to a struct
func (m GLRYmongoPersisterImpl) Update(ctx context.Context, query bson.M, update interface{}, meta map[string]interface{}) *gf_core.Gf_error {

	elem := reflect.TypeOf(update).Elem()
	val := reflect.ValueOf(update).Elem()
	if _, ok := elem.FieldByName("LastUpdatedF"); ok {
		f := val.FieldByName("LastUpdatedF")
		if f.CanSet() {
			now := float64(time.Now().UnixNano()) / 1000000000.0
			f.SetFloat(now)
		}
	}

	return gf_core.Mongo__upsert(query, update, meta, m.Runtime.DB.MongoDB.Collection(m.CollNameStr), ctx, m.Runtime.RuntimeSys)
	// result, err := m.Collection.UpdateByID(ctx, id, bson.D{{"$set", update}}, opts...)
	// if err != nil {
	// 	return err
	// }
	// if result.ModifiedCount == 0 || result.MatchedCount == 0 {
	// 	return errors.New("could not find document to update")
	// }

	// return nil
}

// result must be a slice of pointers to the struct of the type expected to be decoded from mongo
func (m GLRYmongoPersisterImpl) Find(ctx context.Context, filter bson.M, result []interface{}, opts *options.FindOptions, meta map[string]interface{}) *gf_core.Gf_error {

	filter["deleted"] = false

	cur, gErr := gf_core.Mongo__find(filter, opts, meta, m.Runtime.DB.MongoDB.Collection(m.CollNameStr), ctx, m.Runtime.RuntimeSys)
	if gErr != nil {
		return gErr
	}
	defer cur.Close(ctx)
	if err := cur.All(ctx, result); err != nil {
		return gf_core.Error__create("nft id not found in query values",
			"mongodb_cursor_all",
			map[string]interface{}{}, err, "glry_db", m.Runtime.RuntimeSys)
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
