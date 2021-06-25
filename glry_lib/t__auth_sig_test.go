package glry_lib

import (
	"fmt"
	"testing"
	// "context"
	"github.com/fatih/color"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	// gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	// "github.com/davecgh/go-spew/spew"
)

//---------------------------------------------------
func TestAuthSignatures(pTest *testing.T) {

	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	log.Info(fmt.Sprint(cyan("TEST__AUTH_SIGNATURES"), yellow(" ==============================================")))
	// ctx := context.Background()

	walletSignaturesLst := []map[string]string{

		//--------------------
		// METAMASK

		// personal.sign() - "Ethereum Signed Message:" header
		map[string]string{
			"name":           "metamask_00",
			"wallet_address": "0xBA47Bef4ca9e8F86149D2f109478c6bd8A642C97",
			"msg":            "test_msg_glry===2222==",
			"signature":      "0xe3c30211b94bdd49980cf3c8a59e49ca00e8a577a08389f0399cfe7fa16ab03359266cccced3459b30a27e3840892b2032783fe33a52fef9ecc008b38366ff7e1c",
			"signature_type": "personal_sign",
		},

		// personal.sign() - "Ethereum Signed Message:" header
		map[string]string{
			"name":           "metamask_0",
			"wallet_address": "0xBA47Bef4ca9e8F86149D2f109478c6bd8A642C97",
			"msg":            "test_msg_glry",
			"signature":      "0x9c1fd58773b877ad2e6713d835002e222834f29993c419d1672696f4b80501800ed3e4497d52f7a50d428803eddd6bbaa7267dca7f1d73e3c13dd85b0e4fa3471c",
			"signature_type": "personal_sign",
		},

		// personal.sign() - "Ethereum Signed Message:" header
		map[string]string{
			"name":           "metamask_1",
			"wallet_address": "0xBA47Bef4ca9e8F86149D2f109478c6bd8A642C97",
			"msg":            "test_msg_glry=====",
			"signature":      "0xc3e229612b88f9e1bf91e865c57136f1aec5866abbd1135ba8d3cfc0ec8e640d06c678ff9f494937de95a4c0612280a3bf1c19fe1b1fbfb77626e72d03d1ac021c",
			"signature_type": "personal_sign",
		},

		// personal.sign() - "Ethereum Signed Message:" header
		map[string]string{
			"name":           "metamask_2",
			"wallet_address": "0x4e6Dde64f6cd29294282000214Fe586b9112739B",
			"msg":            "testNonceValue",
			"signature":      "0xc5902a1a3102cb9e89e58def4fc2c16d324cc2c668c3b19fca126721a7f85b2a4201d7d573ce52c92e03f93aa95e12da1efd668d9461dc1a2c4cb98cff06a7781c",
			"signature_type": "personal_sign",
		},

		// personal.sign() - "Ethereum Signed Message:" header
		map[string]string{
			"name":           "metamask_32",
			"wallet_address": "0x4e6Dde64f6cd29294282000214Fe586b9112739B",
			"msg":            "test_msg_glry",
			"signature":      "0xcf2060cdd95fce605a7b249924aa5e5e76800bbd4a2d2324d54d038b8fe19b901386605a096f8c05c3788041d464d14b4441931702460d402963099af72d63421b",
			"signature_type": "personal_sign",
		},

		//--------------------
		// WALLET_CONNECT

		// no header
		map[string]string{
			"name":           "wallet_connect",
			"wallet_address": "0x4e6Dde64f6cd29294282000214Fe586b9112739B",
			"msg":            "testNonceValue",
			"signature":      "0xdb84030e15aff945d28d069fc1eb29b2f069a39e224dab2dd07fc5cec13c4fcf3fb726fc1bc5b796404abd0a91b3ff185c78a55492b33d8e1386907d4eef603b1c",
			"signature_type": "eth_sign",
		},

		// no header
		map[string]string{
			"name":           "wallet_connect",
			"wallet_address": "0x4e6Dde64f6cd29294282000214Fe586b9112739B",
			"msg":            "test_msg_glry",
			"signature":      "0x6283b1305f2aba08f5ea3183c56e66d16b136d00dbce274e1c7ac43290a389e76dcb6c4cc790680497a0371bd70ced1582b2822d415acf0489da64390ea3d3b21c",
			"signature_type": "eth_sign",
		},

		//--------------------
		/*// WALLET_LINK
		map[string]string{
			"name":           "wallet_link",
			"wallet_address": "0x4e6Dde64f6cd29294282000214Fe586b9112739B",
			"msg":            "testNonceValue",
			"signature":      "0x44a9326081fa0fde15d1a07a975bde947b5230312857c6c4c0a21f077a87e10f34e4d7f907debb5f67881f1bae893e58ec573890dd49eace61a5a65dde940b5f1b",
			"signature_type": "eth_sign",
		},*/

		//--------------------
	}

	// spew.Dump(walletSignaturesLst)

	//--------------------
	// RUNTIME_SYS
	mongoURLstr := "mongodb://127.0.0.1:27017"
	mongoDBnameStr := "glry_test"
	config := &glry_core.GLRYconfig{
		// Env            string
		// BaseURL        string
		// WebBaseURL     string
		// Port              int
		MongoURLstr:       mongoURLstr,
		MongoDBnameStr:    mongoDBnameStr,
		JWTtokenTTLsecInt: 86400,
	}
	runtime, gErr := glry_core.RuntimeGet(config)
	if gErr != nil {
		pTest.Fail()
	}

	//--------------------

	// ADD!! - test negative cases as well, where incorrect signatures are passed it
	//         and a failure is expected.
	for _, v := range walletSignaturesLst {

		testSignatureStr := v["signature"]
		testWalletAddressStr := v["wallet_address"]
		testMsgStr := v["msg"]

		fmt.Println("============================")
		fmt.Println(v["name"])
		fmt.Println(testSignatureStr)

		// VERIFY
		validBool, gErr := AuthVerifySignatureAllMethods(testSignatureStr,
			testMsgStr,
			glry_db.GLRYuserAddress(testWalletAddressStr),
			runtime)
		if gErr != nil {
			pTest.Fail()
		}

		log.WithFields(log.Fields{"valid": validBool}).Info("signature validity")

		assert.True(pTest, validBool, "test signature is not valid")
	}
}
