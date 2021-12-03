package mongodb

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/gammazero/workerpool"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsoncodec"
	"go.mongodb.org/mongo-driver/bson/bsonrw"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const (
	galleryDBName = "gallery"
)

const bsonDateFormat = "2006-01-02T15:04:05.999Z"

var (
	collectionUnassignedTTL time.Duration = time.Hour * 24
	openseaAssetsTTL        time.Duration = time.Minute * 5
	galleriesTTL            time.Duration = time.Hour * 24
)

var updateQueueDefaultTimeout = time.Second * 10

var errInvalidValue = errors.New("cannot encode invalid element")

var addressType = reflect.TypeOf(persist.Address(""))

var creationTimeType = reflect.TypeOf(persist.CreationTime(time.Time{}))

var lastUpdatedTimeType = reflect.TypeOf(persist.LastUpdatedTime(time.Time{}))

var tokenMetadataType = reflect.TypeOf(persist.TokenMetadata{})

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

type updateModel struct {
	query   bson.M
	setDocs interface{}
}

type errNotStruct struct {
	iAmNotAStruct interface{}
}

// NewMongoClient returns a new MongoClient instance
func NewMongoClient(ctx context.Context, mOpts *options.ClientOptions) *mongo.Client {

	mClient, err := mongo.Connect(ctx, mOpts)
	if err != nil {
		panic(err)
	}

	err = mClient.Ping(ctx, readpref.Primary())
	if err != nil {
		panic(err)
	}

	return mClient
}

// newStorage returns a new MongoStorage instance with a pointer to a collection of the specified name
// and the specified version
func newStorage(mongoClient *mongo.Client, version int64, dbName, collName string) *storage {
	coll := mongoClient.Database(dbName).Collection(collName)

	return &storage{version: version, collection: coll}
}

// Insert inserts a document into the mongo database while filling out the fields id, creation time, and last updated
func (m *storage) insert(ctx context.Context, insert interface{}, opts ...*options.InsertOneOptions) (persist.DBID, error) {
	defer util.Track("mongo.Insert", time.Now())
	res, err := m.collection.InsertOne(ctx, insert, opts...)
	if err != nil {
		return "", err
	}

	return persist.DBID(res.InsertedID.(string)), nil
}

// InsertMany inserts many documents into a mongo database while filling out the fields id, creation time, and last updated for each
func (m *storage) insertMany(ctx context.Context, insert []interface{}, opts ...*options.InsertManyOptions) ([]persist.DBID, error) {
	defer util.Track("mongo.InsertMany", time.Now())
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
	defer util.Track("mongo.Update", time.Now())
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
	defer util.Track("mongo.Push", time.Now())
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
	defer util.Track("mongo.PullAll", time.Now())
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
	defer util.Track("mongo.Pull", time.Now())
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
	defer util.Track("mongo.Upsert", time.Now())
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

// bulkUpsert upserts many documents in the mongo database while filling out the fields id, creation time, and last updated
func (m *storage) bulkUpdate(ctx context.Context, updates []updateModel, isUpsert bool) error {
	defer util.Track("mongo.BulkUpdate", time.Now())
	wp := workerpool.New(10)
	errs := make(chan error)

	for i := 0; i < len(updates); i += 50 {
		var toUpdate []updateModel
		if i+50 < len(updates) {
			toUpdate = updates[i : i+50]
		} else {
			toUpdate = updates[i:]
		}

		updateModels := make([]mongo.WriteModel, len(toUpdate))
		for i, upsert := range toUpdate {

			model := &mongo.UpdateOneModel{
				Filter: upsert.query,
				Update: upsert.setDocs,
				Upsert: &isUpsert,
			}

			updateModels[i] = model
		}

		wp.Submit(func() {
			beginUpsert := time.Now()
			res, err := m.collection.BulkWrite(ctx, updateModels, options.BulkWrite().SetOrdered(false))
			if err != nil {
				errs <- err
				return
			}
			logrus.Infof("upserted %d documents and modified %d documents in %s", res.UpsertedCount, res.ModifiedCount, time.Since(beginUpsert))
		})

	}

	go func() {
		defer close(errs)
		wp.StopWait()
		errs <- nil
	}()

	if err := <-errs; err != nil {
		return err
	}

	return nil
}

// find finds documents in the mongo database which is not deleted
// result must be a slice of pointers to the struct of the type expected to be decoded from mongo
func (m *storage) find(ctx context.Context, filter bson.M, result interface{}, opts ...*options.FindOptions) error {
	defer util.Track("mongo.Find", time.Now())

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
	defer util.Track("mongo.Aggregate", time.Now())
	cur, err := m.collection.Aggregate(ctx, agg, opts...)
	if err != nil {
		return err
	}

	return cur.All(ctx, result)

}

// count counts the number of documents in the mongo database which is not deleted
func (m *storage) count(ctx context.Context, filter bson.M, opts ...*options.CountOptions) (int64, error) {
	defer util.Track("mongo.Count", time.Now())
	if len(filter) == 0 {
		return m.collection.EstimatedDocumentCount(ctx)
	}
	filter["deleted"] = false
	return m.collection.CountDocuments(ctx, filter, opts...)
}

// delete deletes all documents matching a given filter query
func (m *storage) delete(ctx context.Context, filter bson.M, opts ...*options.DeleteOptions) error {
	defer util.Track("mongo.Delete", time.Now())
	_, err := m.collection.DeleteMany(ctx, filter, opts...)
	return err
}

// createIndex creates a new index in the mongo database
func (m *storage) createIndex(ctx context.Context, index mongo.IndexModel, opts ...*options.CreateIndexesOptions) (string, error) {
	defer util.Track("mongo.CreateIndex", time.Now())
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

func tokenMetadataEncodeValue(ec bsoncodec.EncodeContext, vw bsonrw.ValueWriter, val reflect.Value) error {
	if !val.IsValid() || val.Kind() != reflect.Map || val.Type().Key().Kind() != reflect.String || val.Type() != tokenMetadataType {
		return bsoncodec.ValueEncoderError{Name: "MetadataEncodeValue", Types: []reflect.Type{tokenMetadataType}, Kinds: []reflect.Kind{reflect.Map}, Received: val}
	}

	if val.IsNil() {
		// If we have a nill map but we can't WriteNull, that means we're probably trying to encode
		// to a TopLevel document. We can't currently tell if this is what actually happened, but if
		// there's a deeper underlying problem, the error will also be returned from WriteDocument,
		// so just continue. The operations on a map reflection value are valid, so we can call
		// MapKeys within mapEncodeValue without a problem.
		err := vw.WriteNull()
		if err == nil {
			return nil
		}
	}

	buf := new(bytes.Buffer)
	zipped := gzip.NewWriter(buf)

	asJSON, err := json.Marshal(map[string]interface{}(val.Interface().(persist.TokenMetadata)))
	if err != nil {
		return err
	}

	_, err = zipped.Write(asJSON)
	if err != nil {
		return err
	}
	err = zipped.Close()
	if err != nil {
		return err
	}
	return vw.WriteBinary(buf.Bytes())

	// dw, err := vw.WriteDocument()
	// if err != nil {
	// 	return err
	// }

	// return mapEncodeValue(ec, dw, val, nil)
}

// mapEncodeValue handles encoding of the values of a map. The collisionFn returns
// true if the provided key exists, this is mainly used for inline maps in the
// struct codec.
func mapEncodeValue(ec bsoncodec.EncodeContext, dw bsonrw.DocumentWriter, val reflect.Value, collisionFn func(string) bool) error {

	elemType := val.Type().Elem()
	encoder, err := ec.LookupEncoder(elemType)
	if err != nil && elemType.Kind() != reflect.Interface {
		return err
	}

	keys := val.MapKeys()
	for _, key := range keys {
		if collisionFn != nil && collisionFn(key.String()) {
			return fmt.Errorf("Key %s of inlined map conflicts with a struct field name", key)
		}

		currEncoder, currVal, lookupErr := lookupElementEncoder(ec, encoder, val.MapIndex(key))
		if lookupErr != nil && lookupErr != errInvalidValue {
			return lookupErr
		}

		vw, err := dw.WriteDocumentElement(strings.ReplaceAll(strings.ReplaceAll(key.String(), ".", ""), "$", ""))
		if err != nil {
			return err
		}

		if lookupErr == errInvalidValue {
			err = vw.WriteNull()
			if err != nil {
				return err
			}
			continue
		}

		if enc, ok := currEncoder.(bsoncodec.ValueEncoder); ok {
			err = enc.EncodeValue(ec, vw, currVal)
			if err != nil {
				return err
			}
			continue
		}
		err = encoder.EncodeValue(ec, vw, currVal)
		if err != nil {
			return err
		}
	}

	return dw.WriteDocumentEnd()
}

func tokenMetadataDecodeValue(ec bsoncodec.DecodeContext, vw bsonrw.ValueReader, val reflect.Value) error {
	if !val.IsValid() || !val.CanSet() || val.Type() != tokenMetadataType {
		return bsoncodec.ValueDecoderError{Name: "TokenMetadataDecodeValue", Types: []reflect.Type{tokenMetadataType}, Received: val}
	}
	bin, _, err := vw.ReadBinary()
	if err != nil {
		return err
	}

	asJSON := new(bytes.Buffer)
	unzipper, err := gzip.NewReader(bytes.NewReader(bin))
	if err != nil {
		return err
	}

	_, err = io.Copy(asJSON, unzipper)
	if err != nil {
		return err
	}

	err = unzipper.Close()
	if err != nil {
		return err
	}

	asTokenMetadata := make(persist.TokenMetadata)
	if err = json.Unmarshal(asJSON.Bytes(), &asTokenMetadata); err != nil {
		return err
	}
	val.Set(reflect.ValueOf(asTokenMetadata))

	return nil
}

func lookupElementEncoder(ec bsoncodec.EncodeContext, origEncoder bsoncodec.ValueEncoder, currVal reflect.Value) (bsoncodec.ValueEncoder, reflect.Value, error) {
	if origEncoder != nil || (currVal.Kind() != reflect.Interface) {
		return origEncoder, currVal, nil
	}
	currVal = currVal.Elem()
	if !currVal.IsValid() {
		return nil, currVal, errInvalidValue
	}
	currEncoder, err := ec.LookupEncoder(currVal.Type())

	return currEncoder, currVal, err
}

func creationTimeEncodeValue(ec bsoncodec.EncodeContext, vw bsonrw.ValueWriter, val reflect.Value) error {
	if !val.IsValid() || val.Type() != creationTimeType {
		return bsoncodec.ValueEncoderError{Name: "CreationTimeEncodeValue", Types: []reflect.Type{creationTimeType}, Received: val}
	}
	s := time.Time(val.Interface().(persist.CreationTime))
	primDate := primitive.NewDateTimeFromTime(s)
	if s.IsZero() {
		return vw.WriteDateTime(int64(primitive.NewDateTimeFromTime(time.Now())))
	}
	return vw.WriteDateTime(int64(primDate))
}

func lastUpdatedTimeEncodeValue(ec bsoncodec.EncodeContext, vw bsonrw.ValueWriter, val reflect.Value) error {
	if !val.IsValid() || val.Type() != lastUpdatedTimeType {
		return bsoncodec.ValueEncoderError{Name: "LastUpdatedTimeEncodeValue", Types: []reflect.Type{lastUpdatedTimeType}, Received: val}
	}
	return vw.WriteDateTime(int64(primitive.NewDateTimeFromTime(time.Now())))
}

func dateTimeDecodeValue(ec bsoncodec.DecodeContext, vw bsonrw.ValueReader, val reflect.Value) error {
	if !val.IsValid() || !val.CanSet() || (val.Type() != lastUpdatedTimeType && val.Type() != creationTimeType) {
		return bsoncodec.ValueDecoderError{Name: "DateTimeDecodeValue", Types: []reflect.Type{lastUpdatedTimeType, creationTimeType}, Received: val}
	}
	dt, err := vw.ReadDateTime()
	if err != nil {
		return err
	}
	switch val.Type() {
	case creationTimeType:
		val.Set(reflect.ValueOf(persist.CreationTime(primitive.DateTime(dt).Time())))
	case lastUpdatedTimeType:
		val.Set(reflect.ValueOf(persist.LastUpdatedTime(primitive.DateTime(dt).Time())))
	}
	return nil
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
	rb.RegisterTypeDecoder(lastUpdatedTimeType, bsoncodec.ValueDecoderFunc(dateTimeDecodeValue))
	rb.RegisterTypeDecoder(creationTimeType, bsoncodec.ValueDecoderFunc(dateTimeDecodeValue))
	rb.RegisterTypeEncoder(tokenMetadataType, bsoncodec.ValueEncoderFunc(tokenMetadataEncodeValue))
	rb.RegisterTypeDecoder(tokenMetadataType, bsoncodec.ValueDecoderFunc(tokenMetadataDecodeValue))
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
