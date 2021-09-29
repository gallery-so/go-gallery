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

func getPreflightRequest(assert *assert.Assertions, address string, jwt string) *http.Response {
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/auth/get_preflight?address=%s", tc.serverURL, address),
		nil)
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwt))
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
