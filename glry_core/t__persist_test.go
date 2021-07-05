package glry_core

import (
	"context"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type testDoc struct {
	IDstr         GLRYdbId `bson:"_id,omitempty"`
	CreationTimeF float64  `bson:"creation_time"`
	ImportantData string   `bson:"data"`
	Deleted       bool     `bson:"deleted"`
}

//---------------------------------------------------
func TestPersist(pTest *testing.T) {

	fmt.Println("TEST__PERSIST ==============================================")

	ctx := context.Background()
	if deadline, ok := pTest.Deadline(); ok {
		newCtx, cancel := context.WithDeadline(ctx, deadline)
		defer cancel()
		ctx = newCtx
	}

	//--------------------
	// RUNTIME_SYS

	runtime, gErr := RuntimeGet(&GLRYconfig{MongoURLstr: "mongodb://127.0.0.1:27017", MongoDBnameStr: "gallery", Port: 4000, BaseURL: "http://localhost:4000", EnvStr: "glry_test"})
	if gErr != nil {
		pTest.Fail()
	}

	//--------------------

	m := NewMongoPersister(1, "sub", runtime)

	sub := testDoc{ImportantData: "hype"}

	err := m.Insert(ctx, &sub, &options.InsertOneOptions{})
	if err != nil {
		fmt.Println(err)
		pTest.Fail()
	}

	up := testDoc{ImportantData: "potty"}
	err = m.Update(ctx, bson.M{"data": "hype"}, &up, &options.UpdateOptions{})
	if err != nil {
		fmt.Println(err)
		pTest.Fail()
	}

	resSub := []testDoc{}

	err = m.Find(ctx, bson.M{}, &resSub, &options.FindOptions{})
	if err != nil {
		fmt.Println(err)
		pTest.Fail()
	}

	if len(resSub) == 0 {
		pTest.Fail()
	}

	fmt.Println(resSub)

}
