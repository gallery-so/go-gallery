package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/assert"
)

func TestGetUserByID_Success(t *testing.T) {
	setupTest(t)

	assert := assert.New(t)

	resp, err := http.Get(fmt.Sprintf("%s/users/get?user_id=%s", tc.serverURL, tc.user1.id))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(tc.user1.username, body.UserName)
}

func TestGetUserByAddress_Success(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	resp, err := http.Get(fmt.Sprintf("%s/users/get?address=%s", tc.serverURL, tc.user2.address))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(tc.user2.username, body.UserName)
}

func TestGetUserByUsername_Success(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	resp, err := http.Get(fmt.Sprintf("%s/users/get?username=%s", tc.serverURL, tc.user1.username))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(tc.user1.username, body.UserName)
}

func TestGetUserAuthenticated_ShouldIncludeAddress(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	userID := tc.user1.id
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/users/get?user_id=%s", tc.serverURL, userID), nil)
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(userID, body.ID)
	assert.NotEmpty(body.Addresses)
}

func TestGetUserUnAuthenticated_ShouldNotIncludeAddress(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	userID := tc.user1.id
	resp, err := http.Get(fmt.Sprintf("%s/users/get?user_id=%s", tc.serverURL, userID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(userID, body.ID)
	assert.Empty(body.Addresses)
}

func TestUpdateUserAuthenticated_Success(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	update := userUpdateInput{
		UserNameStr: "kaito",
	}
	resp := updateUserInfoRequest(assert, update, tc.user1.jwt)
	assertValidJSONResponse(assert, resp)

	user, err := persist.UserGetByID(context.Background(), tc.user1.id, tc.r)
	assert.Nil(err)
	assert.Equal(update.UserNameStr, user.UserName)
}

// Updating the username to itself should not trigger an error, despite the DB
// having a user entity with that username already
func TestUpdateUserAuthenticated_NoChange_Success(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	update := userUpdateInput{
		UserNameStr: "bob",
	}
	resp := updateUserInfoRequest(assert, update, tc.user1.jwt)
	assertValidJSONResponse(assert, resp)

	user, err := persist.UserGetByID(context.Background(), tc.user1.id, tc.r)
	assert.Nil(err)
	assert.Equal(update.UserNameStr, user.UserName)
}

func TestUpdateUserUnauthenticated_Failure(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	update := userUpdateInput{
		UserNameStr: "kaito",
	}
	resp := updateUserInfoNoAuthRequest(assert, update)
	assertGalleryErrorResponse(assert, resp)
}

func TestUpdateUserAuthenticated_UsernameTaken_Failure(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	update := userUpdateInput{
		UserNameStr: tc.user2.username,
	}
	resp := updateUserInfoRequest(assert, update, tc.user1.jwt)
	assertGalleryErrorResponse(assert, resp)

	user, err := persist.UserGetByID(context.Background(), tc.user1.id, tc.r)
	assert.Nil(err)
	assert.NotEqual(update.UserNameStr, user.UserName)
}

func updateUserInfoRequest(assert *assert.Assertions, input userUpdateInput, jwt string) *http.Response {
	data, err := json.Marshal(input)
	assert.Nil(err)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/users/update/info", tc.serverURL), bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	return resp
}

func updateUserInfoNoAuthRequest(assert *assert.Assertions, input userUpdateInput) *http.Response {
	data, err := json.Marshal(input)
	assert.Nil(err)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/users/update/info", tc.serverURL), bytes.NewBuffer(data))
	assert.Nil(err)
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	return resp
}
