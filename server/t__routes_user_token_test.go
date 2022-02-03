package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"github.com/mikeydub/go-gallery/service/auth"
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
	assert.Equal(tc.user1.username, body.Username.String())
}

func TestGetUserByAddress_Success_Token(t *testing.T) {
	assert := setupTest(t, 2)

	resp, err := http.Get(fmt.Sprintf("%s/users/get?address=%s", tc.serverURL, tc.user2.address))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(tc.user2.username, body.Username.String())
}

func TestGetUserByUsername_Success_Token(t *testing.T) {
	assert := setupTest(t, 2)

	resp, err := http.Get(fmt.Sprintf("%s/users/get?username=%s", tc.serverURL, tc.user1.username))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(tc.user1.username, body.Username.String())
}

func TestGetUserAuthenticated_ShouldIncludeAddress_Token(t *testing.T) {
	assert := setupTest(t, 2)

	userID := tc.user1.id
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/users/get?user_id=%s", tc.serverURL, userID), nil)
	assert.Nil(err)
	resp, err := tc.user1.client.Do(req)
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(userID, body.ID)
	assert.NotEmpty(body.Addresses)
}

func TestGetCurrentUser_ValidCookieReturnsUser_Token(t *testing.T) {
	assert := setupTest(t, 2)

	// Create user
	u := persist.User{}
	userID, err := tc.repos.UserRepository.Create(context.Background(), u)
	assert.Nil(err)

	// Set up cookie to make an authenticated request
	jwt, err := auth.JWTGeneratePipeline(context.Background(), userID)
	assert.Nil(err)
	tu := createCookieAndSetOnUser(assert, userID, jwt)

	// Make request
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/users/get/current", tc.serverURL), nil)
	assert.Nil(err)

	resp, err := tu.client.Do(req)
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.User{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(tu.id.String(), body.ID.String())
}

func TestGetCurrentUser_NoCookieReturnsNoContent_Token(t *testing.T) {
	assert := setupTest(t, 2)

	resp, err := http.Get(fmt.Sprintf("%s/users/get/current", tc.serverURL))
	assert.Nil(err)
	assert.Equal(http.StatusNoContent, resp.StatusCode)
}

func TestGetCurrentUser_InvalidCookieReturnsNoContent_Token(t *testing.T) {
	assert := setupTest(t, 2)

	// Create user
	u := persist.User{}
	userID, err := tc.repos.UserRepository.Create(context.Background(), u)
	assert.Nil(err)

	// Set up invalid cookie
	tu := createCookieAndSetOnUser(assert, userID, "invalid token")

	// Make request
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/users/get/current", tc.serverURL), nil)
	assert.Nil(err)

	resp, err := tu.client.Do(req)
	assert.Nil(err)
	assert.Equal(http.StatusNoContent, resp.StatusCode)
}

func TestUpdateUserAuthenticated_Success_Token(t *testing.T) {
	assert := setupTest(t, 2)

	update := user.UpdateUserInput{
		UserName: "kaito",
	}
	resp := updateUserInfoRequestToken(assert, update, tc.user1)
	assertValidJSONResponse(assert, resp)

	user, err := tc.repos.UserRepository.GetByID(context.Background(), tc.user1.id)
	assert.Nil(err)
	assert.Equal(update.UserName, user.Username.String())
}

// Updating the username to itself should not trigger an error, despite the DB
// having a user entity with that username already
func TestUpdateUserAuthenticated_NoChange_Success_Token(t *testing.T) {
	assert := setupTest(t, 2)

	update := user.UpdateUserInput{
		UserName: "bob",
	}
	resp := updateUserInfoRequestToken(assert, update, tc.user1)
	assertValidJSONResponse(assert, resp)

	user, err := tc.repos.UserRepository.GetByID(context.Background(), tc.user1.id)
	assert.Nil(err)
	assert.Equal(update.UserName, user.Username.String())
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

	user2, err := tc.repos.UserRepository.GetByID(context.Background(), tc.user2.id)
	assert.Nil(err)
	log.Println(user2.Username)

	update := user.UpdateUserInput{
		UserName: tc.user2.username,
	}
	resp := updateUserInfoRequestToken(assert, update, tc.user1)
	assertErrorResponse(assert, resp)

	user, err := tc.repos.UserRepository.GetByID(context.Background(), tc.user1.id)
	assert.Nil(err)
	assert.NotEqual(update.UserName, user.Username)
}
func TestUpdateUserAuthenticated_UsernameInvalid_Failure_Token(t *testing.T) {
	assert := setupTest(t, 2)

	update := user.UpdateUserInput{
		UserName: "92ks&$m__",
	}
	resp := updateUserInfoRequestToken(assert, update, tc.user1)
	assertErrorResponse(assert, resp)

	user, err := tc.repos.UserRepository.GetByID(context.Background(), tc.user1.id)
	assert.Nil(err)
	assert.NotEqual(update.UserName, user.Username)
}

func TestUserAddAddresses_Success_Token(t *testing.T) {
	assert := setupTest(t, 2)

	nonce := persist.UserNonce{
		Value:   "TestNonce",
		Address: "0x9a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5",
	}
	err := tc.repos.NonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	update := user.AddUserAddressesInput{
		Address:   persist.Address(strings.ToLower("0x9a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5")),
		Signature: "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b",
	}
	resp := userAddAddressesRequestToken(assert, update, tc.user1)
	assertValidJSONResponse(assert, resp)

	errResp := &util.ErrorResponse{}
	err = util.UnmarshallBody(errResp, resp.Body)
	assert.Nil(err)
	assert.Empty(errResp.Error)

	updatedUser, err := tc.repos.UserRepository.GetByID(context.Background(), tc.user1.id)
	assert.Nil(err)
	assert.Equal(update.Address, updatedUser.Addresses[1])
}

func TestUserAddAddresses_WrongNonce_Failure_Token(t *testing.T) {
	assert := setupTest(t, 2)

	nonce := persist.UserNonce{
		Value:   "Wrong Nonce",
		Address: "0x9a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5",
	}
	err := tc.repos.NonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	update := user.AddUserAddressesInput{
		Address:   "0x9a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5",
		Signature: "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b",
	}
	resp := userAddAddressesRequestToken(assert, update, tc.user1)
	assertErrorResponse(assert, resp)
}

func TestUserAddAddresses_OtherUserOwnsAddress_Failure_Token(t *testing.T) {
	assert := setupTest(t, 2)

	u := persist.User{
		Addresses: []persist.Address{"0x9a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5"},
	}
	_, err := tc.repos.UserRepository.Create(context.Background(), u)

	nonce := persist.UserNonce{
		Value:   "TestNonce",
		Address: "0x9a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5",
	}
	err = tc.repos.NonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	update := user.AddUserAddressesInput{
		Address:   "0x9a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5",
		Signature: "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b",
	}
	resp := userAddAddressesRequestToken(assert, update, tc.user1)
	assertErrorResponse(assert, resp)
}

func TestUserRemoveAddresses_Success_Token(t *testing.T) {
	assert := setupTest(t, 2)

	u := persist.User{
		Addresses:          []persist.Address{"0xcb1b78568d0ef81585f074b0dfd6b743959070d9", "0x9a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5"},
		Username:           "TestUser",
		UsernameIdempotent: "testuser",
	}
	userID, err := tc.repos.UserRepository.Create(context.Background(), u)
	assert.Nil(err)

	nft := persist.Token{
		OwnerAddress:    "0x9a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5",
		TokenID:         "10",
		ContractAddress: "0x9a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5",
		Name:            "test",
	}
	nftID, err := tc.repos.TokenRepository.Create(context.Background(), nft)
	assert.Nil(err)

	nft2 := persist.Token{
		OwnerAddress:    "0xcb1b78568d0ef81585f074b0dfd6b743959070d9",
		TokenID:         "11",
		ContractAddress: "0xcb1b78568d0ef81585f074b0dfd6b743959070d9",
		Name:            "test2",
	}
	nftID2, err := tc.repos.TokenRepository.Create(context.Background(), nft2)
	assert.Nil(err)

	coll := persist.CollectionTokenDB{
		NFTs:        []persist.DBID{nftID, nftID2},
		Name:        "test-coll",
		OwnerUserID: userID,
	}
	collID, err := tc.repos.CollectionTokenRepository.Create(context.Background(), coll)

	jwt, err := auth.JWTGeneratePipeline(context.Background(), userID)
	assert.Nil(err)

	update := user.RemoveUserAddressesInput{
		Addresses: []persist.Address{"0xcb1b78568d0ef81585f074b0dfd6b743959070d9"},
	}
	j, err := cookiejar.New(nil)
	client := &http.Client{Jar: j}
	tu := &TestUser{
		id:     userID,
		jwt:    jwt,
		client: client,
	}
	getFakeCookie(assert, jwt, client)

	resp := userRemoveAddressesRequestToken(assert, update, tu)
	assertValidJSONResponse(assert, resp)

	errResp := &util.ErrorResponse{}
	util.UnmarshallBody(errResp, resp.Body)
	assert.Empty(errResp.Error)

	nfts, err := tc.repos.TokenRepository.GetByUserID(context.Background(), userID, -1, 0)
	assert.Nil(err)
	assert.Len(nfts, 1)

	resultColl, err := tc.repos.CollectionTokenRepository.GetByID(context.Background(), collID, true)
	assert.Nil(err)
	assert.Len(resultColl.NFTs, 1)

	user, err := tc.repos.UserRepository.GetByID(context.Background(), userID)
	assert.Nil(err)
	assert.Len(user.Addresses, 1)

}

func TestUserRemoveAddresses_NotOwnAddress_Failure_Token(t *testing.T) {
	assert := setupTest(t, 2)

	u := persist.User{
		Addresses: []persist.Address{"0x9a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5", "0xcb1b78568d0ef81585f074b0dfd6b743959070d9"},
	}
	userID, err := tc.repos.UserRepository.Create(context.Background(), u)
	assert.Nil(err)

	jwt, err := auth.JWTGeneratePipeline(context.Background(), userID)
	assert.Nil(err)

	update := user.RemoveUserAddressesInput{
		Addresses: []persist.Address{tc.user1.address},
	}

	j, err := cookiejar.New(nil)
	client := &http.Client{Jar: j}
	tu := &TestUser{
		id:     userID,
		jwt:    jwt,
		client: client,
	}
	getFakeCookie(assert, jwt, client)

	resp := userRemoveAddressesRequestToken(assert, update, tu)
	assertErrorResponse(assert, resp)

}

func TestUserRemoveAddresses_AllAddresses_Failure_Token(t *testing.T) {
	assert := setupTest(t, 2)

	u := persist.User{
		Addresses: []persist.Address{"0x9a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5", "0xcb1b78568d0ef81585f074b0dfd6b743959070d9"},
	}
	userID, err := tc.repos.UserRepository.Create(context.Background(), u)
	assert.Nil(err)

	jwt, err := auth.JWTGeneratePipeline(context.Background(), userID)
	assert.Nil(err)

	update := user.RemoveUserAddressesInput{
		Addresses: u.Addresses,
	}
	j, err := cookiejar.New(nil)
	client := &http.Client{Jar: j}
	tu := &TestUser{
		id:     userID,
		jwt:    jwt,
		client: client,
	}
	getFakeCookie(assert, jwt, client)

	resp := userRemoveAddressesRequestToken(assert, update, tu)
	assertErrorResponse(assert, resp)

}

func createCookieAndSetOnUser(assert *assert.Assertions, userID persist.DBID, jwt string) *TestUser {
	j, err := cookiejar.New(nil)
	assert.Nil(err)
	client := &http.Client{Jar: j}
	tu := &TestUser{
		id:     userID,
		jwt:    jwt,
		client: client,
	}
	getFakeCookie(assert, jwt, client)
	return tu
}

func updateUserInfoRequestToken(assert *assert.Assertions, input user.UpdateUserInput, tu *TestUser) *http.Response {
	data, err := json.Marshal(input)
	assert.Nil(err)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/users/update/info", tc.serverURL), bytes.NewBuffer(data))
	assert.Nil(err)

	resp, err := tu.client.Do(req)
	assert.Nil(err)
	return resp
}
func userAddAddressesRequestToken(assert *assert.Assertions, input user.AddUserAddressesInput, tu *TestUser) *http.Response {
	data, err := json.Marshal(input)
	assert.Nil(err)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/users/update/addresses/add", tc.serverURL), bytes.NewBuffer(data))
	assert.Nil(err)

	resp, err := tu.client.Do(req)
	assert.Nil(err)
	return resp
}

func userRemoveAddressesRequestToken(assert *assert.Assertions, input user.RemoveUserAddressesInput, tu *TestUser) *http.Response {
	data, err := json.Marshal(input)
	assert.Nil(err)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/users/update/addresses/remove", tc.serverURL), bytes.NewBuffer(data))
	assert.Nil(err)

	resp, err := tu.client.Do(req)
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
