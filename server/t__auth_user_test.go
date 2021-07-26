package server

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

//---------------------------------------------------
func TestAuthUser(pTest *testing.T) {

	addressStr := "0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15"

	ctx := context.Background()

	//--------------------
	// USER_GET_PREFLIGHT

	userGetPublicInfoInput := &authUserGetPreflightInput{
		AddressStr: addressStr,
	}
	output, err := authUserGetPreflightDb(userGetPublicInfoInput, ctx, r)
	if err != nil {
		pTest.FailNow()
	}

	nonceStr := output.NonceStr

	assert.NotEmpty(pTest, nonceStr)

	//--------------------
	// USER_CREATE
	userCreateInput := &userCreateInput{
		AddressStr:   addressStr,
		SignatureStr: "how to make this? can we sign the nonce from go?",
	}
	user, err := userCreateDb(userCreateInput, ctx, r)
	if err != nil {
		pTest.Fail()
	}

	//--------------------
	// USER_DELETE

	err = userDeleteDb(user.UserIDstr, ctx, r)
	if err != nil {
		pTest.Fail()
	}

	//--------------------

	log.WithFields(log.Fields{"nonce": nonceStr}).Info("signature validity")

}
