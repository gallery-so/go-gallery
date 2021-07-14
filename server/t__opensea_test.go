package server

import (
	"fmt"
	"testing"
	// gfcore "github.com/gloflow/gloflow/go/gf_core"
)

//---------------------------------------------------
func TestFetchAssertsForAcc(pTest *testing.T) {

	fmt.Println("TEST__OPENSEA ==============================================")

	// TODO
	// ctx := context.Background()

	// //--------------------
	// // RUNTIME_SYS

	// runtime, gErr := runtime.RuntimeGet("127.0.0.1:27017", "glry_test")
	// if gErr != nil {
	// 	pTest.Fail()
	// }

	// //--------------------
	// ownerWalletAddressStr := "0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15"
	// assetsForAccLst, gErr := OpenSeaPipelineAssetsForAcc(ownerWalletAddressStr, ctx, runtime.RuntimeSys)
	// if gErr != nil {
	// 	pTest.Fail()
	// }

	// assert.True(pTest, len(assetsForAccLst) > 0, "more then 0 OpenSea assets should be fetched for Account")

	// spew.Dump(assetsForAccLst)

}
