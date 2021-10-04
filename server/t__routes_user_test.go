package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	assert := setupTest(t)

	resp, err := http.Get(fmt.Sprintf("%s/users/get?address=%s", tc.serverURL, tc.user2.address))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(tc.user2.username, body.UserName)
}

func TestGetUserByUsername_Success(t *testing.T) {
	assert := setupTest(t)

	resp, err := http.Get(fmt.Sprintf("%s/users/get?username=%s", tc.serverURL, tc.user1.username))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(tc.user1.username, body.UserName)
}

func TestGetUserAuthenticated_ShouldIncludeAddress(t *testing.T) {
	assert := setupTest(t)

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
	assert := setupTest(t)

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
	assert := setupTest(t)

	update := userUpdateInput{
		UserName: "kaito",
	}
	resp := updateUserInfoRequest(assert, update, tc.user1.jwt)
	assertValidJSONResponse(assert, resp)

	user, err := persist.UserGetByID(context.Background(), tc.user1.id, tc.r)
	assert.Nil(err)
	assert.Equal(update.UserName, user.UserName)
}

// Updating the username to itself should not trigger an error, despite the DB
// having a user entity with that username already
func TestUpdateUserAuthenticated_NoChange_Success(t *testing.T) {
	assert := setupTest(t)

	update := userUpdateInput{
		UserName: "bob",
	}
	resp := updateUserInfoRequest(assert, update, tc.user1.jwt)
	assertValidJSONResponse(assert, resp)

	user, err := persist.UserGetByID(context.Background(), tc.user1.id, tc.r)
	assert.Nil(err)
	assert.Equal(update.UserName, user.UserName)
}

func TestUpdateUserUnauthenticated_Failure(t *testing.T) {
	assert := setupTest(t)

	update := userUpdateInput{
		UserName: "kaito",
	}
	resp := updateUserInfoNoAuthRequest(assert, update)
	assertErrorResponse(assert, resp)
}

func TestUpdateUserAuthenticated_UsernameTaken_Failure(t *testing.T) {
	assert := setupTest(t)

	update := userUpdateInput{
		UserName: tc.user2.username,
	}
	resp := updateUserInfoRequest(assert, update, tc.user1.jwt)
	assertErrorResponse(assert, resp)

	user, err := persist.UserGetByID(context.Background(), tc.user1.id, tc.r)
	assert.Nil(err)
	assert.NotEqual(update.UserName, user.UserName)
}
func TestUpdateUserAuthenticated_UsernameInvalid_Failure(t *testing.T) {
	assert := setupTest(t)

	update := userUpdateInput{
		UserName: "92ks&$m__",
	}
	resp := updateUserInfoRequest(assert, update, tc.user1.jwt)
	assertErrorResponse(assert, resp)

	user, err := persist.UserGetByID(context.Background(), tc.user1.id, tc.r)
	assert.Nil(err)
	assert.NotEqual(update.UserName, user.UserName)
}

func TestUserAddAddresses_Success(t *testing.T) {
	assert := setupTest(t)

	nonce := &persist.UserNonce{
		Value:   "TestNonce",
		Address: strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31"),
	}
	_, err := persist.AuthNonceCreate(context.Background(), nonce, tc.r)
	assert.Nil(err)

	update := userAddAddressInput{
		Address:   strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31"),
		Signature: "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b",
	}
	resp := userAddAddressesRequest(assert, update, tc.user1.jwt)
	assertValidJSONResponse(assert, resp)

	errResp := &util.ErrorResponse{}
	err = util.UnmarshallBody(errResp, resp.Body)
	assert.Nil(err)
	assert.Empty(errResp.Error)

	updatedUser, err := persist.UserGetByID(context.Background(), tc.user1.id, tc.r)
	assert.Nil(err)
	assert.Equal(update.Address, updatedUser.Addresses[1])
}

func TestUserAddAddresses_WrongNonce_Failure(t *testing.T) {
	assert := setupTest(t)

	nonce := &persist.UserNonce{
		Value:   "Wrong Nonce",
		Address: strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31"),
	}
	_, err := persist.AuthNonceCreate(context.Background(), nonce, tc.r)
	assert.Nil(err)

	update := userAddAddressInput{
		Address:   strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31"),
		Signature: "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b",
	}
	resp := userAddAddressesRequest(assert, update, tc.user1.jwt)
	assertErrorResponse(assert, resp)
}

func TestUserAddAddresses_OtherUserOwnsAddress_Failure(t *testing.T) {
	assert := setupTest(t)

	user := &persist.User{
		Addresses: []string{strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31")},
	}
	_, err := persist.UserCreate(context.Background(), user, tc.r)

	nonce := &persist.UserNonce{
		Value:   "TestNonce",
		Address: strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31"),
	}
	_, err = persist.AuthNonceCreate(context.Background(), nonce, tc.r)
	assert.Nil(err)

	update := userAddAddressInput{
		Address:   strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31"),
		Signature: "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b",
	}
	resp := userAddAddressesRequest(assert, update, tc.user1.jwt)
	assertErrorResponse(assert, resp)
}

func TestUserRemoveAddresses_Success(t *testing.T) {
	assert := setupTest(t)

	user := &persist.User{
		Addresses:          []string{strings.ToLower("0xcb1b78568d0Ef81585f074b0Dfd6B743959070D9"), strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31")},
		UserName:           "TestUser",
		UserNameIdempotent: "testuser",
	}
	userID, err := persist.UserCreate(context.Background(), user, tc.r)
	assert.Nil(err)

	nft := &persist.Token{
		OwnerAddress:   strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31"),
		CollectorsNote: "test",
	}
	nftID, err := persist.TokenCreate(context.Background(), nft, tc.r)

	coll := &persist.CollectionDB{
		Nfts:        []persist.DBID{nftID},
		Name:        "test-coll",
		OwnerUserID: userID,
	}
	collID, err := persist.CollCreate(context.Background(), coll, tc.r)

	jwt, err := jwtGeneratePipeline(context.Background(), userID, tc.r)
	assert.Nil(err)

	update := userRemoveAddressesInput{
		Addresses: []string{strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31")},
	}
	resp := userRemoveAddressesRequest(assert, update, jwt)
	assertValidJSONResponse(assert, resp)

	errResp := &util.ErrorResponse{}
	util.UnmarshallBody(errResp, resp.Body)
	assert.Empty(errResp.Error)

	nfts, err := persist.TokenGetByUserID(context.Background(), userID, 0, 50, tc.r)
	assert.Nil(err)
	assert.Empty(nfts)

	colls, err := persist.CollGetByID(context.Background(), collID, true, tc.r)
	assert.Nil(err)
	assert.NotEmpty(colls)
	assert.Empty(colls[0].Nfts)

}

func TestUserRemoveAddresses_NotOwnAddress_Failure(t *testing.T) {
	assert := setupTest(t)

	user := &persist.User{
		Addresses: []string{strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31"), strings.ToLower("0xcb1b78568d0Ef81585f074b0Dfd6B743959070D9")},
	}
	userID, err := persist.UserCreate(context.Background(), user, tc.r)
	assert.Nil(err)

	jwt, err := jwtGeneratePipeline(context.Background(), userID, tc.r)
	assert.Nil(err)

	update := userRemoveAddressesInput{
		Addresses: []string{strings.ToLower(tc.user1.address)},
	}

	resp := userRemoveAddressesRequest(assert, update, jwt)
	assertErrorResponse(assert, resp)

}

func TestUserRemoveAddresses_AllAddresses_Failure(t *testing.T) {
	assert := setupTest(t)

	user := &persist.User{
		Addresses: []string{strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31"), strings.ToLower("0xcb1b78568d0Ef81585f074b0Dfd6B743959070D9")},
	}
	userID, err := persist.UserCreate(context.Background(), user, tc.r)
	assert.Nil(err)

	jwt, err := jwtGeneratePipeline(context.Background(), userID, tc.r)
	assert.Nil(err)

	update := userRemoveAddressesInput{
		Addresses: user.Addresses,
	}

	resp := userRemoveAddressesRequest(assert, update, jwt)
	assertErrorResponse(assert, resp)

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
func userAddAddressesRequest(assert *assert.Assertions, input userAddAddressInput, jwt string) *http.Response {
	data, err := json.Marshal(input)
	assert.Nil(err)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/users/update/addresses/add", tc.serverURL), bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	return resp
}

func userRemoveAddressesRequest(assert *assert.Assertions, input userRemoveAddressesInput, jwt string) *http.Response {
	data, err := json.Marshal(input)
	assert.Nil(err)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/users/update/addresses/remove", tc.serverURL), bytes.NewBuffer(data))
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
