package glry_lib

import (
	"fmt"
	"testing"
	"context"
	"github.com/fatih/color"
	// "github.com/stretchr/testify/assert"
	// gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/davecgh/go-spew/spew"
)

//---------------------------------------------------
func TestAuthSignatures(pTest *testing.T) {

	cyan   := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	fmt.Println(cyan("TEST__AUTH_SIGNATURES"), yellow(" =============================================="))
	

	ctx := context.Background()

	fmt.Println(ctx)




	/*
	metamask
	wallet address: "0x4e6Dde64f6cd29294282000214Fe586b9112739B"
	nonce:          "testNonceValue"
	signature:      "0xc5902a1a3102cb9e89e58def4fc2c16d324cc2c668c3b19fca126721a7f85b2a4201d7d573ce52c92e03f93aa95e12da1efd668d9461dc1a2c4cb98cff06a7781c"

	wallet connect
	wallet address: "0x4e6Dde64f6cd29294282000214Fe586b9112739B"
	nonce: "testNonceValue"
	signature: "0xdb84030e15aff945d28d069fc1eb29b2f069a39e224dab2dd07fc5cec13c4fcf3fb726fc1bc5b796404abd0a91b3ff185c78a55492b33d8e1386907d4eef603b1c"

	wallet link
	wallet address: "0x4e6Dde64f6cd29294282000214Fe586b9112739B"
	nonce:          "testNonceValue"
	signature:      "0x44a9326081fa0fde15d1a07a975bde947b5230312857c6c4c0a21f077a87e10f34e4d7f907debb5f67881f1bae893e58ec573890dd49eace61a5a65dde940b5f1b"
	*/
	wallet_signatures_map := map[string]map[string]string{
		"metamask": map[string]string{
			"wallet_address": "0x4e6Dde64f6cd29294282000214Fe586b9112739B",
			"nonce":          "testNonceValue",
			"signature":      "0xc5902a1a3102cb9e89e58def4fc2c16d324cc2c668c3b19fca126721a7f85b2a4201d7d573ce52c92e03f93aa95e12da1efd668d9461dc1a2c4cb98cff06a7781c",
		},
	}

	spew.Dump(wallet_signatures_map)

	//--------------------
	// RUNTIME_SYS

	mongodbHostStr := "127.0.0.1:27017"
	runtime, gErr := glry_core.RuntimeGet(mongodbHostStr, "glry_test")
	if gErr != nil {
		pTest.Fail()
	}

	fmt.Println(runtime)
	
	//--------------------

	// assert.True(pTest, len(assetsForAccLst) > 0, "more then 0 OpenSea assets should be fetched for Account")
	




	testSignatureStr := wallet_signatures_map["metamask"]["signature"]
	testNonceStr     := wallet_signatures_map["metamask"]["nonce"]
	testPublicKeyStr := wallet_signatures_map["metamask"]["wallet_address"]

	validBool, gErr := AuthUserVerifySignature(testSignatureStr, testNonceStr, testPublicKeyStr, runtime)
	if gErr != nil {
		pTest.Fail()
	}








	fmt.Println(validBool)
	






}