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

	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/user"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/assert"
)

func TestGetUserByID_Success_Token(t *testing.T) {
	setupTest(t, 2)

	assert := assert.New(t)

	resp, err := http.Get(fmt.Sprintf("%s/users/get?user_id=%s", tc.serverURL, tc.user1.id))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(tc.user1.username, body.Username)
}

func TestGetUserByAddress_Success_Token(t *testing.T) {
	assert := setupTest(t, 2)

	resp, err := http.Get(fmt.Sprintf("%s/users/get?address=%s", tc.serverURL, tc.user2.address))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(tc.user2.username, body.Username)
}

func TestGetUserByUsername_Success_Token(t *testing.T) {
	assert := setupTest(t, 2)

	resp, err := http.Get(fmt.Sprintf("%s/users/get?username=%s", tc.serverURL, tc.user1.username))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(tc.user1.username, body.Username)
}

func TestGetUserAuthenticated_ShouldIncludeAddress_Token(t *testing.T) {
	assert := setupTest(t, 2)

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

func TestUpdateUserAuthenticated_Success_Token(t *testing.T) {
	assert := setupTest(t, 2)

	update := user.UpdateUserInput{
		UserName: "kaito",
	}
	resp := updateUserInfoRequestToken(assert, update, tc.user1.jwt)
	assertValidJSONResponse(assert, resp)

	user, err := tc.repos.userRepository.GetByID(context.Background(), tc.user1.id)
	assert.Nil(err)
	assert.Equal(update.UserName, user.Username)
}

// Updating the username to itself should not trigger an error, despite the DB
// having a user entity with that username already
func TestUpdateUserAuthenticated_NoChange_Success_Token(t *testing.T) {
	assert := setupTest(t, 2)

	update := user.UpdateUserInput{
		UserName: "bob",
	}
	resp := updateUserInfoRequestToken(assert, update, tc.user1.jwt)
	assertValidJSONResponse(assert, resp)

	user, err := tc.repos.userRepository.GetByID(context.Background(), tc.user1.id)
	assert.Nil(err)
	assert.Equal(update.UserName, user.Username)
}

func TestUpdateUserUnauthenticated_Failure_Token(t *testing.T) {
	assert := setupTest(t, 2)

	update := user.UpdateUserInput{
		UserName: "kaito",
	}
	resp := updateUserInfoNoAuthRequestToken(assert, update)
	assertErrorResponse(assert, resp)
}

func TestUpdateUserAuthenticated_UsernameTaken_Failure_Token(t *testing.T) {
	assert := setupTest(t, 2)

	user2, err := tc.repos.userRepository.GetByID(context.Background(), tc.user2.id)
	assert.Nil(err)
	log.Println(user2.Username)

	update := user.UpdateUserInput{
		UserName: tc.user2.username,
	}
	resp := updateUserInfoRequestToken(assert, update, tc.user1.jwt)
	assertErrorResponse(assert, resp)

	user, err := tc.repos.userRepository.GetByID(context.Background(), tc.user1.id)
	assert.Nil(err)
	assert.NotEqual(update.UserName, user.Username)
}
func TestUpdateUserAuthenticated_UsernameInvalid_Failure_Token(t *testing.T) {
	assert := setupTest(t, 2)

	update := user.UpdateUserInput{
		UserName: "92ks&$m__",
	}
	resp := updateUserInfoRequestToken(assert, update, tc.user1.jwt)
	assertErrorResponse(assert, resp)

	user, err := tc.repos.userRepository.GetByID(context.Background(), tc.user1.id)
	assert.Nil(err)
	assert.NotEqual(update.UserName, user.Username)
}

func TestUserAddAddresses_Success_Token(t *testing.T) {
	assert := setupTest(t, 2)

	nonce := persist.UserNonce{
		Value:   "TestNonce",
		Address: "0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5",
	}
	err := tc.repos.nonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	update := user.AddUserAddressesInput{
		Address:   persist.Address(strings.ToLower("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5")),
		Signature: "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b",
	}
	resp := userAddAddressesRequestToken(assert, update, tc.user1.jwt)
	assertValidJSONResponse(assert, resp)

	errResp := &util.ErrorResponse{}
	err = util.UnmarshallBody(errResp, resp.Body)
	assert.Nil(err)
	assert.Empty(errResp.Error)

	updatedUser, err := tc.repos.userRepository.GetByID(context.Background(), tc.user1.id)
	assert.Nil(err)
	assert.Equal(update.Address, updatedUser.Addresses[1])
}

func TestUserAddAddresses_WrongNonce_Failure_Token(t *testing.T) {
	assert := setupTest(t, 2)

	nonce := persist.UserNonce{
		Value:   "Wrong Nonce",
		Address: "0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5",
	}
	err := tc.repos.nonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	update := user.AddUserAddressesInput{
		Address:   "0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5",
		Signature: "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b",
	}
	resp := userAddAddressesRequestToken(assert, update, tc.user1.jwt)
	assertErrorResponse(assert, resp)
}

func TestUserAddAddresses_OtherUserOwnsAddress_Failure_Token(t *testing.T) {
	assert := setupTest(t, 2)

	u := persist.User{
		Addresses: []persist.Address{"0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5"},
	}
	_, err := tc.repos.userRepository.Create(context.Background(), u)

	nonce := persist.UserNonce{
		Value:   "TestNonce",
		Address: "0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5",
	}
	err = tc.repos.nonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	update := user.AddUserAddressesInput{
		Address:   "0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5",
		Signature: "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b",
	}
	resp := userAddAddressesRequestToken(assert, update, tc.user1.jwt)
	assertErrorResponse(assert, resp)
}

func TestUserRemoveAddresses_Success_Token(t *testing.T) {
	assert := setupTest(t, 2)

	u := persist.User{
		Addresses:          []persist.Address{"0xcb1b78568d0Ef81585f074b0Dfd6B743959070D9", "0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5"},
		Username:           "TestUser",
		UsernameIdempotent: "testuser",
	}
	userID, err := tc.repos.userRepository.Create(context.Background(), u)
	assert.Nil(err)

	nft := persist.Token{
		OwnerAddress: "0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5",
		Name:         "test",
	}
	nftID, err := tc.repos.tokenRepository.Create(context.Background(), nft)

	coll := persist.CollectionTokenDB{
		NFTs:        []persist.DBID{nftID},
		Name:        "test-coll",
		OwnerUserID: userID,
	}
	collID, err := tc.repos.collectionTokenRepository.Create(context.Background(), coll)

	jwt, err := middleware.JWTGeneratePipeline(context.Background(), userID)
	assert.Nil(err)

	update := user.RemoveUserAddressesInput{
		Addresses: []persist.Address{"0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5"},
	}
	resp := userRemoveAddressesRequestToken(assert, update, jwt)
	assertValidJSONResponse(assert, resp)

	errResp := &util.ErrorResponse{}
	util.UnmarshallBody(errResp, resp.Body)
	assert.Empty(errResp.Error)

	nfts, err := tc.repos.nftRepository.GetByUserID(context.Background(), userID)
	assert.Nil(err)
	assert.Empty(nfts)

	resultColl, err := tc.repos.collectionRepository.GetByID(context.Background(), collID, true)
	assert.Nil(err)
	assert.Empty(resultColl.NFTs)

}

func TestUserRemoveAddresses_NotOwnAddress_Failure_Token(t *testing.T) {
	assert := setupTest(t, 2)

	u := persist.User{
		Addresses: []persist.Address{"0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5", "0xcb1b78568d0Ef81585f074b0Dfd6B743959070D9"},
	}
	userID, err := tc.repos.userRepository.Create(context.Background(), u)
	assert.Nil(err)

	jwt, err := middleware.JWTGeneratePipeline(context.Background(), userID)
	assert.Nil(err)

	update := user.RemoveUserAddressesInput{
		Addresses: []persist.Address{tc.user1.address},
	}

	resp := userRemoveAddressesRequestToken(assert, update, jwt)
	assertErrorResponse(assert, resp)

}

func TestUserRemoveAddresses_AllAddresses_Failure_Token(t *testing.T) {
	assert := setupTest(t, 2)

	u := persist.User{
		Addresses: []persist.Address{"0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5", "0xcb1b78568d0Ef81585f074b0Dfd6B743959070D9"},
	}
	userID, err := tc.repos.userRepository.Create(context.Background(), u)
	assert.Nil(err)

	jwt, err := middleware.JWTGeneratePipeline(context.Background(), userID)
	assert.Nil(err)

	update := user.RemoveUserAddressesInput{
		Addresses: u.Addresses,
	}

	resp := userRemoveAddressesRequestToken(assert, update, jwt)
	assertErrorResponse(assert, resp)

}

func updateUserInfoRequestToken(assert *assert.Assertions, input user.UpdateUserInput, jwt string) *http.Response {
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
func userAddAddressesRequestToken(assert *assert.Assertions, input user.AddUserAddressesInput, jwt string) *http.Response {
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

func userRemoveAddressesRequestToken(assert *assert.Assertions, input user.RemoveUserAddressesInput, jwt string) *http.Response {
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

func updateUserInfoNoAuthRequestToken(assert *assert.Assertions, input user.UpdateUserInput) *http.Response {
	data, err := json.Marshal(input)
	assert.Nil(err)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/users/update/info", tc.serverURL), bytes.NewBuffer(data))
	assert.Nil(err)
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	return resp
}
