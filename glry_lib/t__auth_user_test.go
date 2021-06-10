package glry_lib

import (
	"fmt"
	"testing"
	"context"
	"github.com/fatih/color"
	log "github.com/sirupsen/logrus"
	// "github.com/stretchr/testify/assert"
	// gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	// "github.com/davecgh/go-spew/spew"
)

//---------------------------------------------------
func TestAuthUser(pTest *testing.T) {

	cyan   := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	fmt.Println(cyan("TEST__AUTH_USER"), yellow("=============================================="))
	
	addressStr := glry_db.GLRYuserAddress("0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15")

	ctx := context.Background()

	//--------------------
	// RUNTIME_SYS

	mongoURLstr    := "mongodb://127.0.0.1:27017"
	mongoDBnameStr := "glry_test"
	config := &glry_core.GLRYconfig {
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
	// USER_GET_PREFLIGHT

	userGetPublicInfoInput := &GLRYauthUserGetPreflightInput{
		AddressStr: user.AddressesLst[0],
	}
	output, gErr := AuthUserGetPreflightPipeline(userGetPublicInfoInput, ctx, runtime)
	if gErr != nil {
		pTest.Fail()
	}

	nonceStr := output.NonceStr

	//--------------------
	// USER_CREATE
	userCreateInput := &GLRYauthUserCreateInput{
		
		SignatureStr:  ,
		AddressStr:    addressStr,
		NonceValueStr: nonceStr,
	}
	user, gErr := AuthUserCreatePipeline(userCreateInput, ctx, runtime)
	if gErr != nil {
		pTest.Fail()
	}

	// spew.Dump(user)

	//--------------------
	// USER_DELETE

	gErr = AuthUserDeletePipeline(user.IDstr, ctx, runtime)
	if gErr != nil {
		pTest.Fail()
	}
	
	//--------------------



	log.WithFields(log.Fields{"nonce": nonceStr,}).Info("signature validity")
	fmt.Println()

	// assert.True(pTest, len(assetsForAccLst) > 0, "more then 0 OpenSea assets should be fetched for Account")
	






}