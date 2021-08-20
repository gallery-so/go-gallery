package server

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

//---------------------------------------------------
func TestAuthUser(pTest *testing.T) {

	address := "0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15"

	ctx := context.Background()

	//--------------------
	// USER_GET_PREFLIGHT

	userGetPublicInfoInput := &authUserGetPreflightInput{
		Address: address,
	}
	output, err := authUserGetPreflightDb(ctx, userGetPublicInfoInput, tc.r)
	if err != nil {
		pTest.FailNow()
	}

	nonce := output.Nonce

	assert.NotEmpty(pTest, nonce)

	//--------------------
	// USER_CREATE
	userCreateInput := &userAddAddressInput{
		Address:   address,
		Signature: "how to make this? can we sign the nonce from go?",
	}
	user, err := userCreateDb(ctx, userCreateInput, tc.r)
	if err != nil {
		pTest.Fail()
	}

	//--------------------
	// USER_DELETE

	// TODO why segfault?
	err = userDeleteDb(ctx, user.UserID, tc.r)
	if err != nil {
		pTest.Fail()
	}

	//--------------------

	log.WithFields(log.Fields{"nonce": nonce}).Info("signature validity")

}
