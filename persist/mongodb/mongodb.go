package mongodb

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/mikeydub/go-gallery/persist"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsoncodec"
	"go.mongodb.org/mongo-driver/bson/bsonrw"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	galleryDBName = "gallery"
)

const bsonDateFormat = "2006-01-02T15:04:05.999Z"

var (
	collectionUnassignedTTL time.Duration = time.Minute * 1
	openseaAssetsTTL        time.Duration = time.Minute * 5
)

var addressType = reflect.TypeOf(persist.Address(""))

var creationTimeType = reflect.TypeOf(persist.CreationTime(time.Time{}))

var lastUpdatedTimeType = reflect.TypeOf(persist.LastUpdatedTime(time.Time{}))

var idType = reflect.TypeOf(persist.GenerateID())

// CustomRegistry is the custom mongo BSON encoding/decoding registry
var CustomRegistry = createCustomRegistry().Build()

// ErrDocumentNotFound represents when a document is not found in the database for an update operation
var ErrDocumentNotFound = errors.New("document not found")

// storage represents the currently accessed collection and the version of the "schema"
type storage struct {
	version    int64
	collection *mongo.Collection
}

type errNotStruct struct {
	iAmNotAStruct interface{}
}

// newStorage returns a new MongoStorage instance with a pointer to a collection of the specified name
// and the specified version
func newStorage(mongoClient *mongo.Client, version int64, dbName, collName string) *storage {
	coll := mongoClient.Database(dbName).Collection(collName)

	return &storage{version: version, collection: coll}
}

// Insert inserts a document into the mongo database while filling out the fields id, creation time, and last updated
func (m *storage) insert(ctx context.Context, insert interface{}, opts ...*options.InsertOneOptions) (persist.DBID, error) {

	res, err := m.collection.InsertOne(ctx, insert, opts...)
	if err != nil {
		return "", err
	}

	return persist.DBID(res.InsertedID.(string)), nil
}

// InsertMany inserts many documents into a mongo database while filling out the fields id, creation time, and last updated for each
func (m *storage) insertMany(ctx context.Context, insert []interface{}, opts ...*options.InsertManyOptions) ([]persist.DBID, error) {

	res, err := m.collection.InsertMany(ctx, insert, opts...)
	if err != nil {
		return nil, err
	}

	ids := make([]persist.DBID, len(res.InsertedIDs))

	for i, v := range res.InsertedIDs {
		if id, ok := v.(string); ok {
			ids[i] = persist.DBID(id)
		}
	}
	return ids, nil
}

// Update updates a document in the mongo database while filling out the field LastUpdated
func (m *storage) update(ctx context.Context, query bson.M, update interface{}, opts ...*options.UpdateOptions) error {

	result, err := m.collection.UpdateMany(ctx, query, bson.D{{Key: "$set", Value: update}}, opts...)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrDocumentNotFound
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
		return ErrDocumentNotFound
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
		return ErrDocumentNotFound
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
		return ErrDocumentNotFound
	}

	return nil
}

// Upsert upserts a document in the mongo database while filling out the fields id, creation time, and last updated
func (m *storage) upsert(ctx context.Context, query bson.M, upsert interface{}, opts ...*options.UpdateOptions) (persist.DBID, error) {

	var returnID persist.DBID
	opts = append(opts, &options.UpdateOptions{Upsert: boolin(true)})
	now := primitive.NewDateTimeFromTime(time.Now())
	asBSON, err := bson.MarshalWithRegistry(CustomRegistry, upsert)
	if err != nil {
		return returnID, err
	}

	asMap := bson.M{}
	err = bson.UnmarshalWithRegistry(CustomRegistry, asBSON, &asMap)
	if err != nil {
		return returnID, err
	}
	asMap["last_updated"] = now
	if _, ok := asMap["created_at"]; !ok {
		asMap["created_at"] = now
	}

	if id, ok := asMap["_id"]; ok && id != "" {
		returnID = persist.DBID(id.(string))
	}

	delete(asMap, "_id")
	for k := range query {
		delete(asMap, k)
	}
	res, err := m.collection.UpdateOne(ctx, query, bson.M{"$setOnInsert": bson.M{"_id": persist.GenerateID()}, "$set": asMap}, opts...)
	if err != nil {
		return "", err
	}

	if it, ok := res.UpsertedID.(string); ok {
		returnID = persist.DBID(it)
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
func (m *storage) count(ctx context.Context, filter bson.M, opts ...*options.CountOptions) (int64, error) {
	if len(filter) == 0 {
		return m.collection.EstimatedDocumentCount(ctx)
	}
	filter["deleted"] = false
	return m.collection.CountDocuments(ctx, filter, opts...)
}

// delete deletes all documents matching a given filter query
func (m *storage) delete(ctx context.Context, filter bson.M, opts ...*options.DeleteOptions) error {
	_, err := m.collection.DeleteMany(ctx, filter, opts...)
	return err
}

// createIndex creates a new index in the mongo database
func (m *storage) createIndex(ctx context.Context, index mongo.IndexModel, opts ...*options.CreateIndexesOptions) (string, error) {
	return m.collection.Indexes().CreateOne(ctx, index, opts...)
}

// function that returns the pointer to the bool passed in
func boolin(b bool) *bool {
	return &b
}

func addressEncodeValue(ec bsoncodec.EncodeContext, vw bsonrw.ValueWriter, val reflect.Value) error {
	if !val.IsValid() || val.Type() != addressType {
		return bsoncodec.ValueEncoderError{Name: "AddressEncodeValue", Types: []reflect.Type{addressType}, Received: val}
	}
	s := val.Interface().(persist.Address).String()
	return vw.WriteString(s)
}

func creationTimeEncodeValue(ec bsoncodec.EncodeContext, vw bsonrw.ValueWriter, val reflect.Value) error {
	if !val.IsValid() || val.Type() != creationTimeType {
		return bsoncodec.ValueEncoderError{Name: "CreationTimeEncodeValue", Types: []reflect.Type{creationTimeType}, Received: val}
	}
	s := time.Time(val.Interface().(persist.CreationTime))
	if s.IsZero() {
		return vw.WriteDateTime(time.Now().UnixNano() / int64(time.Millisecond))
	}
	return vw.WriteDateTime(s.UnixNano() / int64(time.Millisecond))
}

func lastUpdatedTimeEncodeValue(ec bsoncodec.EncodeContext, vw bsonrw.ValueWriter, val reflect.Value) error {
	if !val.IsValid() || val.Type() != lastUpdatedTimeType {
		return bsoncodec.ValueEncoderError{Name: "LastUpdatedTimeEncodeValue", Types: []reflect.Type{lastUpdatedTimeType}, Received: val}
	}
	return vw.WriteDateTime(time.Now().UnixNano() / int64(time.Millisecond))
}

func idEncodeValue(ec bsoncodec.EncodeContext, vw bsonrw.ValueWriter, val reflect.Value) error {
	if !val.IsValid() || val.Type() != idType {
		return bsoncodec.ValueEncoderError{Name: "IDEncodeValue", Types: []reflect.Type{idType}, Received: val}
	}
	s := val.Interface().(persist.DBID)

	if s == "" {
		s = persist.GenerateID()
	}
	return vw.WriteString(string(s))

}

func createCustomRegistry() *bsoncodec.RegistryBuilder {
	var primitiveCodecs bson.PrimitiveCodecs
	rb := bsoncodec.NewRegistryBuilder()
	bsoncodec.DefaultValueEncoders{}.RegisterDefaultEncoders(rb)
	bsoncodec.DefaultValueDecoders{}.RegisterDefaultDecoders(rb)
	rb.RegisterTypeEncoder(addressType, bsoncodec.ValueEncoderFunc(addressEncodeValue))
	rb.RegisterTypeEncoder(idType, bsoncodec.ValueEncoderFunc(idEncodeValue))
	rb.RegisterTypeEncoder(creationTimeType, bsoncodec.ValueEncoderFunc(creationTimeEncodeValue))
	rb.RegisterTypeEncoder(lastUpdatedTimeType, bsoncodec.ValueEncoderFunc(lastUpdatedTimeEncodeValue))
	primitiveCodecs.RegisterPrimitiveCodecs(rb)
	return rb
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

func (e errNotStruct) Error() string {
	return fmt.Sprintf("%v is not a struct, is of type %T", e.iAmNotAStruct, e.iAmNotAStruct)
}
