package glry_lib

import (
	"fmt"
	"testing"
	"context"
	"github.com/fatih/color"
	// "github.com/stretchr/testify/assert"
	// gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	"github.com/davecgh/go-spew/spew"
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

	mongodbHostStr := "127.0.0.1:27017"
	runtime, gErr := glry_core.RuntimeGet(mongodbHostStr, "glry_test")
	if gErr != nil {
		pTest.Fail()
	}

	//--------------------
	// USER_CREATE
	userCreateInput := &GLRYauthUserCreateInput{
		NameStr:    "test_user",
		AddressStr: addressStr,
	}
	user, gErr := AuthUserCreatePipeline(userCreateInput, ctx, runtime)
	if gErr != nil {
		pTest.Fail()
	}


	spew.Dump(user)

	//--------------------
	// USER_GET_PUBLIC_INFO

	userGetPublicInfoInput := &GLRYauthUserGetPublicInfoInput{
		AddressStr: user.AddressesLst[0],
	}
	nonceInt, gErr := AuthUserGetPublicInfoPipeline(userGetPublicInfoInput, ctx, runtime)
	if gErr != nil {
		pTest.Fail()
	}

	//--------------------
	// USER_DELETE

	gErr = AuthUserDeletePipeline(user.IDstr, ctx, runtime)
	if gErr != nil {
		pTest.Fail()
	}
	



	//--------------------


	fmt.Println(nonceInt)

	// assert.True(pTest, len(assetsForAccLst) > 0, "more then 0 OpenSea assets should be fetched for Account")
	






}