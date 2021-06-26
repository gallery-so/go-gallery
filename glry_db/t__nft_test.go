package glry_db

import (
	"context"
	"fmt"
	"testing"
	"time"

	// gfcore "github.com/gloflow/gloflow/go/gf_core"

	"github.com/mikeydub/go-gallery/glry_core"
	"go.mongodb.org/mongo-driver/bson"
)

//---------------------------------------------------
func TestCreateAndGetNFT(pTest *testing.T) {

	fmt.Println("TEST__NFT ==============================================")

	ctx := context.Background()
	if deadline, ok := pTest.Deadline(); ok {
		newCtx, cancel := context.WithDeadline(ctx, deadline)
		defer cancel()
		ctx = newCtx
	}

	//--------------------
	// RUNTIME_SYS

	runtime, gErr := glry_core.RuntimeGet(&glry_core.GLRYconfig{MongoURLstr: "mongodb://127.0.0.1:27017", MongoDBnameStr: "gallery", Port: 4000, BaseURL: "http://localhost:4000", EnvStr: "glry_test"})
	if gErr != nil {
		pTest.Fail()
	}

	//--------------------

	ownerWalletAddressStr := "0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15"
	name := "Testing"
	creationTime := float64(time.Now().Unix())
	id := NFTcreateID(name, ownerWalletAddressStr, creationTime)
	nft := &GLRYnft{
		IDstr:          id,
		NameStr:        name,
		CreationTimeF:  creationTime,
		DescriptionStr: "A really cool nft",
	}

	gErr = NFTcreate(nft, ctx, runtime)
	if gErr != nil {
		pTest.Fail()
	}

	results, gErr := NFTgetByID(string(id), ctx, runtime)
	if gErr != nil {
		pTest.Fail()
	}
	if len(results) == 0 {
		pTest.Fail()
	}

	res, err := runtime.DB.MongoDB.Collection("glry_nfts").DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		pTest.Fail()
	}
	if res.DeletedCount == 0 {
		pTest.Fail()
	}
}
