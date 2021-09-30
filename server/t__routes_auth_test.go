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

func TestAuthPreflightUserExists_Success(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	resp := getPreflightRequest(assert, tc.user1.address, tc.user1.jwt)
	assertValidResponse(assert, resp)

	type PreflightResp struct {
		authUserGetPreflightOutput
		Error string `json:"error"`
	}
	output := &PreflightResp{}
	err := util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.Empty(output.Error)
	assert.True(output.UserExists)
}

func TestAuthPreflightUserNotExists_Success(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	resp := getPreflightRequest(assert, "0x456d569592f15Af845D0dbe984C12BAB8F430e31", "")
	assertValidResponse(assert, resp)

	type PreflightResp struct {
		authUserGetPreflightOutput
		Error string `json:"error"`
	}
	output := &PreflightResp{}
	err := util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.Empty(output.Error)
	assert.False(output.UserExists)
}

func TestAuthPreflightUserNotExistWithJWT_Success(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	resp := getPreflightRequest(assert, "0x456d569592f15Af845D0dbe984C12BAB8F430e31", tc.user1.jwt)
	assertValidResponse(assert, resp)

	type PreflightResp struct {
		authUserGetPreflightOutput
		Error string `json:"error"`
	}
	output := &PreflightResp{}
	err := util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.Empty(output.Error)
	assert.False(output.UserExists)
}

func TestUserCreate_Success(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	nonce := &persist.UserNonce{
		Value:   "TestNonce",
		Address: strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31"),
	}
	_, err := persist.AuthNonceCreate(context.Background(), nonce, tc.r)
	assert.Nil(err)

	resp := createUserRequest(assert, "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b", nonce.Address)
	assertValidResponse(assert, resp)

	type UserCreateOutput struct {
		userCreateOutput
		Error string `json:"error"`
	}
	output := &UserCreateOutput{}
	err = util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.Empty(output.Error)
	assert.True(output.SignatureValid)
	assert.NotEmpty(output.UserID)
}
func TestUserCreate_WrongNonce_Failure(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	nonce := &persist.UserNonce{
		Value:   "Wrong Nonce",
		Address: strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31"),
	}
	_, err := persist.AuthNonceCreate(context.Background(), nonce, tc.r)
	assert.Nil(err)

	resp := createUserRequest(assert, "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b", nonce.Address)
	assertErrorResponse(assert, resp)
}

func TestUserCreate_WrongSig_Failure(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	nonce := &persist.UserNonce{
		Value:   "TestNonce",
		Address: strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31"),
	}
	_, err := persist.AuthNonceCreate(context.Background(), nonce, tc.r)
	assert.Nil(err)

	resp := createUserRequest(assert, "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb808s191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b", nonce.Address)
	assertErrorResponse(assert, resp)
}

func TestUserCreate_WrongAddress_Failure(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	nonce := &persist.UserNonce{
		Value:   "TestNonce",
		Address: strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e32"),
	}
	_, err := persist.AuthNonceCreate(context.Background(), nonce, tc.r)
	assert.Nil(err)

	resp := createUserRequest(assert, "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b", nonce.Address)
	assertErrorResponse(assert, resp)
}

func TestUserCreate_NoNonce_Failure(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	resp := createUserRequest(assert, "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b", "0x456d569592f15Af845D0dbe984C12BAB8F430e32")
	assertErrorResponse(assert, resp)
}

func TestUserLogin_Success(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	user := &persist.User{
		Addresses: []string{strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31")},
	}

	userID, err := persist.UserCreate(context.Background(), user, tc.r)
	assert.Nil(err)

	nonce := &persist.UserNonce{
		Value:   "TestNonce",
		UserID:  userID,
		Address: user.Addresses[0],
	}
	_, err = persist.AuthNonceCreate(context.Background(), nonce, tc.r)
	assert.Nil(err)

	resp := loginRequest(assert, "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b", nonce.Address)
	assertValidResponse(assert, resp)

	type LoginOutput struct {
		authUserLoginOutput
		Error string `json:"error"`
	}
	output := &LoginOutput{}
	err = util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.Empty(output.Error)
	assert.True(output.SignatureValid)
	assert.NotEmpty(output.UserID)
}

func TestUserLogin_WrongNonce_Failure(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	user := &persist.User{
		Addresses: []string{strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31")},
	}

	userID, err := persist.UserCreate(context.Background(), user, tc.r)
	assert.Nil(err)

	nonce := &persist.UserNonce{
		Value:   "Wrong Nonce",
		UserID:  userID,
		Address: user.Addresses[0],
	}
	_, err = persist.AuthNonceCreate(context.Background(), nonce, tc.r)
	assert.Nil(err)

	resp := loginRequest(assert, "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b", nonce.Address)
	assertErrorResponse(assert, resp)
}

func TestUserLogin_WrongSig_Failure(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	user := &persist.User{
		Addresses: []string{strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31")},
	}

	userID, err := persist.UserCreate(context.Background(), user, tc.r)
	assert.Nil(err)

	nonce := &persist.UserNonce{
		Value:   "TestNonce",
		UserID:  userID,
		Address: user.Addresses[0],
	}
	_, err = persist.AuthNonceCreate(context.Background(), nonce, tc.r)
	assert.Nil(err)

	resp := loginRequest(assert, "0x0a22246c5feee38a80dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b", nonce.Address)
	assertErrorResponse(assert, resp)
}

func TestUserLogin_WrongAddr_Failure(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	user := &persist.User{
		Addresses: []string{strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31")},
	}

	userID, err := persist.UserCreate(context.Background(), user, tc.r)
	assert.Nil(err)

	nonce := &persist.UserNonce{
		Value:   "TestNonce",
		UserID:  userID,
		Address: user.Addresses[0],
	}
	_, err = persist.AuthNonceCreate(context.Background(), nonce, tc.r)
	assert.Nil(err)

	resp := loginRequest(assert, "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b", "0xcb1b78568d0Ef81585f074b0Dfd6B743959070D9")
	assertErrorResponse(assert, resp)
}

func TestUserLogin_NoNonce_Failure(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	user := &persist.User{
		Addresses: []string{strings.ToLower("0x456d569592f15Af845D0dbe984C12BAB8F430e31")},
	}

	_, err := persist.UserCreate(context.Background(), user, tc.r)
	assert.Nil(err)

	resp := loginRequest(assert, "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b", "0x456d569592f15Af845D0dbe984C12BAB8F430e31")
	assertErrorResponse(assert, resp)
}

func TestUserLogin_UserNotExist_Failure(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	nonce := &persist.UserNonce{
		Value:   "TestNonce",
		Address: "0x456d569592f15Af845D0dbe984C12BAB8F430e31",
	}
	_, err := persist.AuthNonceCreate(context.Background(), nonce, tc.r)
	assert.Nil(err)

	resp := loginRequest(assert, "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b", nonce.Address)
	assertErrorResponse(assert, resp)
}

func TestUserLogin_UserNotOwnAddress_Failure(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	nonce := &persist.UserNonce{
		Value:   "TestNonce",
		UserID:  tc.user1.id,
		Address: "0x456d569592f15Af845D0dbe984C12BAB8F430e31",
	}
	_, err := persist.AuthNonceCreate(context.Background(), nonce, tc.r)
	assert.Nil(err)

	resp := loginRequest(assert, "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b", nonce.Address)
	assertErrorResponse(assert, resp)
}

func getPreflightRequest(assert *assert.Assertions, address string, jwt string) *http.Response {
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/auth/get_preflight?address=%s", tc.serverURL, address),
		nil)
	assert.Nil(err)
	if jwt != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwt))
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	return resp
}

func createUserRequest(assert *assert.Assertions, sig, address string) *http.Response {
	body := map[string]string{"address": address, "signature": sig}
	asJSON, err := json.Marshal(body)
	assert.Nil(err)
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/users/create", tc.serverURL),
		bytes.NewReader(asJSON))
	assert.Nil(err)
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	return resp
}

func loginRequest(assert *assert.Assertions, sig, address string) *http.Response {
	body := map[string]string{"address": address, "signature": sig}
	asJSON, err := json.Marshal(body)
	assert.Nil(err)
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/users/login", tc.serverURL),
		bytes.NewReader(asJSON))
	assert.Nil(err)
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	return resp
}
