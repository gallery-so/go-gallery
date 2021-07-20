package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/mikeydub/go-gallery/runtime"
	log "github.com/sirupsen/logrus"
)

//---------------------------------------------------
func TestAuthUser(pTest *testing.T) {

	addressStr := "0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15"

	ctx := context.Background()

	//--------------------
	// RUNTIME_SYS

	mongoURLstr := "mongodb://127.0.0.1:27017"
	mongoDBnameStr := "glry_test"
	config := &runtime.Config{
		// Env            string
		// BaseURL        string
		// WebBaseURL     string
		// Port              int
		MongoURLstr:       mongoURLstr,
		MongoDBnameStr:    mongoDBnameStr,
		JWTtokenTTLsecInt: 86400,
	}

	runtime, gErr := runtime.RuntimeGet(config)
	if gErr != nil {
		pTest.Fail()
	}

	//--------------------
	// USER_GET_PREFLIGHT

	userGetPublicInfoInput := &authUserGetPreflightInput{
		AddressStr: addressStr,
	}
	output, err := authUserGetPreflightDb(userGetPublicInfoInput, ctx, runtime)
	if err != nil {
		pTest.Fail()
	}

	nonceStr := output.NonceStr

	//--------------------
	// USER_CREATE
	userCreateInput := &userCreateInput{
		AddressStr:    addressStr,
		NonceValueStr: nonceStr,
	}
	user, err := userCreateDb(userCreateInput, ctx, runtime)
	if err != nil {
		pTest.Fail()
	}

	//--------------------
	// USER_DELETE

	err = userDeleteDb(user.UserIDstr, ctx, runtime)
	if err != nil {
		pTest.Fail()
	}

	//--------------------

	log.WithFields(log.Fields{"nonce": nonceStr}).Info("signature validity")
	fmt.Println()

}
