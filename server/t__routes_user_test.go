package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
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

func TestUpdateUserAuthenticated_Success(t *testing.T) {
	assert := setupTest(t)

	update := userUpdateInput{
		UserName: "kaito",
	}
	resp := updateUserInfoRequest(assert, update, tc.user1.jwt)
	assertValidJSONResponse(assert, resp)

	user, err := tc.repos.userRepository.GetByID(context.Background(), tc.user1.id)
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

	user, err := tc.repos.userRepository.GetByID(context.Background(), tc.user1.id)
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

	user2, err := tc.repos.userRepository.GetByID(context.Background(), tc.user2.id)
	assert.Nil(err)
	log.Println(user2.UserName)

	update := userUpdateInput{
		UserName: tc.user2.username,
	}
	resp := updateUserInfoRequest(assert, update, tc.user1.jwt)
	assertErrorResponse(assert, resp)

	user, err := tc.repos.userRepository.GetByID(context.Background(), tc.user1.id)
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

	user, err := tc.repos.userRepository.GetByID(context.Background(), tc.user1.id)
	assert.Nil(err)
	assert.NotEqual(update.UserName, user.UserName)
}

func TestUserAddAddresses_Success(t *testing.T) {
	assert := setupTest(t)

	nonce := &persist.UserNonce{
		Value:   "TestNonce",
		Address: strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31"),
	}
	err := tc.repos.nonceRepository.Create(context.Background(), nonce)
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

	updatedUser, err := tc.repos.userRepository.GetByID(context.Background(), tc.user1.id)
	assert.Nil(err)
	assert.Equal(update.Address, updatedUser.Addresses[1])
}

func TestUserAddAddresses_WrongNonce_Failure(t *testing.T) {
	assert := setupTest(t)

	nonce := &persist.UserNonce{
		Value:   "Wrong Nonce",
		Address: strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31"),
	}
	err := tc.repos.nonceRepository.Create(context.Background(), nonce)
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
	_, err := tc.repos.userRepository.Create(context.Background(), user)

	nonce := &persist.UserNonce{
		Value:   "TestNonce",
		Address: strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31"),
	}
	err = tc.repos.nonceRepository.Create(context.Background(), nonce)
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
	userID, err := tc.repos.userRepository.Create(context.Background(), user)
	assert.Nil(err)

	nft := &persist.Token{
		OwnerAddress: strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31"),
		Name:         "test",
	}
	nftID, err := tc.repos.tokenRepository.Create(context.Background(), nft)

	coll := &persist.CollectionDB{
		Nfts:        []persist.DBID{nftID},
		Name:        "test-coll",
		OwnerUserID: userID,
	}
	collID, err := tc.repos.collectionRepository.Create(context.Background(), coll)

	jwt, err := jwtGeneratePipeline(context.Background(), userID)
	assert.Nil(err)

	update := userRemoveAddressesInput{
		Addresses: []string{strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31")},
	}
	resp := userRemoveAddressesRequest(assert, update, jwt)
	assertValidJSONResponse(assert, resp)

	errResp := &util.ErrorResponse{}
	util.UnmarshallBody(errResp, resp.Body)
	assert.Empty(errResp.Error)

	nfts, err := tc.repos.tokenRepository.GetByUserID(context.Background(), userID)
	assert.Nil(err)
	assert.Empty(nfts)

	colls, err := tc.repos.collectionRepository.GetByID(context.Background(), collID, true)
	assert.Nil(err)
	assert.NotEmpty(colls)
	assert.Empty(colls[0].Nfts)

}

func TestUserRemoveAddresses_NotOwnAddress_Failure(t *testing.T) {
	assert := setupTest(t)

	user := &persist.User{
		Addresses: []string{strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31"), strings.ToLower("0xcb1b78568d0Ef81585f074b0Dfd6B743959070D9")},
	}
	userID, err := tc.repos.userRepository.Create(context.Background(), user)
	assert.Nil(err)

	jwt, err := jwtGeneratePipeline(context.Background(), userID)
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
	userID, err := tc.repos.userRepository.Create(context.Background(), user)
	assert.Nil(err)

	jwt, err := jwtGeneratePipeline(context.Background(), userID)
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
