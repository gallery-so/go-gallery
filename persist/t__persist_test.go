package persist

import (
	"context"
	"testing"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type testGrandchild struct {
	IDstr         DbId    `bson:"_id,omitempty"`
	CreationTimeF float64 `bson:"creation_time"`
	ImportantData string  `bson:"data"`
	Deleted       bool    `bson:"deleted"`
}

type testChild struct {
	IDstr         DbId     `bson:"_id,omitempty"`
	CreationTimeF float64  `bson:"creation_time"`
	ImportantData string   `bson:"data"`
	Deleted       bool     `bson:"deleted"`
	Children      []string `bson:"children"`
}

type testGrandparent struct {
	IDstr         DbId     `bson:"_id,omitempty"`
	CreationTimeF float64  `bson:"creation_time"`
	ImportantData string   `bson:"data"`
	Deleted       bool     `bson:"deleted"`
	Children      []string `bson:"children"`
}

//---------------------------------------------------
func TestPersist(pTest *testing.T) {

	pTest.Log("TEST__PERSIST ==============================================")

	ctx := context.Background()
	if deadline, ok := pTest.Deadline(); ok {
		newCtx, cancel := context.WithDeadline(ctx, deadline)
		defer cancel()
		ctx = newCtx
	}

	//--------------------
	// RUNTIME_SYS

	runtime, gErr := runtime.RuntimeGet(&runtime.Config{MongoURLstr: "mongodb://127.0.0.1:27017", MongoDBnameStr: "gallery", Port: 4000, BaseURL: "http://localhost:4000", EnvStr: "glry_test"})
	if gErr != nil {
		pTest.Fail()
	}

	//--------------------

	m := NewMongoStorage(1, "grand_children_collection", runtime)

	sub := testGrandchild{ImportantData: "hype"}

	_, err := m.Insert(ctx, &sub, &options.InsertOneOptions{})
	if err != nil {
		pTest.Log(err)
		pTest.Fail()
	}

	up := testGrandchild{ImportantData: "potty"}
	err = m.Update(ctx, bson.M{"data": "ima childs child"}, &up, &options.UpdateOptions{})
	if err != nil {
		pTest.Log(err)
		pTest.Fail()
	}

	resSub := []testGrandchild{}

	err = m.Find(ctx, bson.M{}, &resSub, options.Find())
	if err != nil {
		pTest.Log(err)
		pTest.Fail()
	}

	if len(resSub) == 0 {
		pTest.Fail()
	}

	pTest.Log(resSub)

	// PARENT

	p := NewMongoStorage(1, "children_collection", runtime)

	parent := testChild{ImportantData: "ima child", Children: []string{string(resSub[0].IDstr)}}

	_, err = p.Insert(ctx, &parent, &options.InsertOneOptions{})
	if err != nil {
		pTest.Log(err)
		pTest.Fail()
	}

	resParent := []testChild{}
	if err := p.Find(ctx, bson.M{}, &resParent); err != nil {
		pTest.Log(err)
		pTest.Fail()
	}
	if len(resParent) == 0 {
		pTest.Fail()
	}

	pTest.Log(resParent)

	// GRANDPARENT

	gp := NewMongoStorage(1, "grand_parents_collection", runtime)

	gparent := testGrandparent{ImportantData: "ima gparent", Children: []string{string(resParent[0].IDstr)}}

	_, err = gp.Insert(ctx, &gparent, &options.InsertOneOptions{})
	if err != nil {
		pTest.Log(err)
		pTest.Fail()
	}

	resGParent := []testGrandparent{}
	if err := gp.Find(ctx, bson.M{}, &resGParent); err != nil {
		pTest.Log(err)
		pTest.Fail()
	}
	if len(resGParent) == 0 {
		pTest.Fail()
	}

	pTest.Log(resGParent)

	// perfect example of mongo pipeling filling in sub documents
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"_id": resGParent[0].IDstr}}},
		{{Key: "$lookup", Value: bson.M{
			"from": "children_collection",
			"let":  bson.M{"childArray": "$children"},
			"pipeline": mongo.Pipeline{
				{{Key: "$match", Value: bson.M{
					"$expr": bson.M{
						"$in": []string{"$_id", "$$childArray"},
					},
				}}},
				{{Key: "$lookup", Value: bson.M{
					"from":         "grand_children_collection",
					"foreignField": "_id",
					"localField":   "children",
					"as":           "children",
				}}},
				{{Key: "$unwind", Value: "$children"}},
			},
			"as": "children",
		}}},
		{{Key: "$unwind", Value: "$children"}},
	}

	// pipeline for a single outer join (e.g. one array of children documents)
	// 	pipeline := mongo.Pipeline{
	// 		{{"$match", bson.M{"_id": id}}},
	// 		{{"$lookup", bson.M{"from": from, "localField": localField, "foreignField": foreignField, "as": as}}},
	// 		{{"$unwind", fmt.Sprintf("$%s", as)}},
	// 	}

	res := []map[string]interface{}{}

	if err := gp.Aggregate(ctx, pipeline, &res); err != nil {
		pTest.Log(err)
		pTest.Fail()
	}

	pTest.Log(res)

	if len(res) == 0 {
		pTest.Fail()
	}
}
