package persist

import (
	"fmt"
	"testing"
	// gfcore "github.com/gloflow/gloflow/go/gf_core"
)

//---------------------------------------------------
func TestCreateAndGetNFT(pTest *testing.T) {

	fmt.Println("TEST__NFT ==============================================")

	// ctx := context.Background()
	// if deadline, ok := pTest.Deadline(); ok {
	// 	newCtx, cancel := context.WithDeadline(ctx, deadline)
	// 	defer cancel()
	// 	ctx = newCtx
	// }

	// //--------------------
	// // RUNTIME_SYS

	// runtime, gErr := runtime.RuntimeGet(&runtime.GLRYconfig{MongoURLstr: "mongodb://127.0.0.1:27017", MongoDBnameStr: "gallery", Port: 4000, BaseURL: "http://localhost:4000", EnvStr: "glry_test"})
	// if gErr != nil {
	// 	pTest.Fail()
	// }

	//--------------------

	// TODO rewrite test to reflect changes to mongo
}
