package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func TestAuthPreflightUserExists_Success(t *testing.T) {
	assert := setupTest(t, 1)

	resp := getPreflightRequest(assert, tc.user1)
	assertValidResponse(assert, resp)

	type PreflightResp struct {
		auth.GetPreflightOutput
		Error string `json:"error"`
	}
	output := &PreflightResp{}
	err := util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.Empty(output.Error)
	assert.True(output.UserExists)
}

func TestAuthPreflightUserNotExists_Success(t *testing.T) {
	assert := setupTest(t, 1)

	resp := getPreflightRequest(assert, &TestUser{address: "0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5", client: &http.Client{}})
	assertValidResponse(assert, resp)

	type PreflightResp struct {
		auth.GetPreflightOutput
		Error string `json:"error"`
	}
	output := &PreflightResp{}
	err := util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.Empty(output.Error)
	assert.False(output.UserExists)
}

func TestAuthPreflightUserNotExistWithJWT_Success(t *testing.T) {
	assert := setupTest(t, 1)

	j, err := cookiejar.New(nil)
	assert.Nil(err)
	client := &http.Client{Jar: j}
	tu := &TestUser{address: "0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5", client: client, jwt: tc.user1.jwt}
	getFakeCookie(assert, tu.jwt, tu.client)
	resp := getPreflightRequest(assert, tu)
	assertValidResponse(assert, resp)

	type PreflightResp struct {
		auth.GetPreflightOutput
		Error string `json:"error"`
	}
	output := &PreflightResp{}
	err = util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.Empty(output.Error)
	assert.False(output.UserExists)
}

func TestUserCreate_Success(t *testing.T) {
	assert := setupTest(t, 1)

	nonce := persist.UserNonce{
		Value:   "TestNonce",
		Address: persist.Address(strings.ToLower("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5")),
	}
	err := tc.repos.NonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	resp := createUserRequest(assert, "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b", nonce.Address)
	assertValidResponse(assert, resp)

	type UserCreateOutput struct {
		user.CreateUserOutput
		Error string `json:"error"`
	}
	output := &UserCreateOutput{}
	err = util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.Empty(output.Error)
	assert.True(output.SignatureValid)
	assert.NotEmpty(output.UserID)
}

func TestUserCreate_UserWithEmptyUsernameExists_Success(t *testing.T) {
	assert := setupTest(t, 1)

	otherUser := persist.User{
		Addresses: []persist.Address{persist.Address(strings.ToLower("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F0AC5"))},
	}
	_, err := tc.repos.UserRepository.Create(context.Background(), otherUser)

	nonce := persist.UserNonce{
		Value:   "TestNonce",
		Address: persist.Address(strings.ToLower("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5")),
	}
	err = tc.repos.NonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	resp := createUserRequest(assert, "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b", nonce.Address)
	assertValidResponse(assert, resp)

	type UserCreateOutput struct {
		user.CreateUserOutput
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
	assert := setupTest(t, 1)

	nonce := persist.UserNonce{
		Value:   "Wrong Nonce",
		Address: persist.Address(strings.ToLower("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5")),
	}
	err := tc.repos.NonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	resp := createUserRequest(assert, "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b", nonce.Address)
	assertErrorResponse(assert, resp)
}

func TestUserCreate_WrongSig_Failure(t *testing.T) {
	assert := setupTest(t, 1)

	nonce := persist.UserNonce{
		Value:   "TestNonce",
		Address: persist.Address(strings.ToLower("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5")),
	}
	err := tc.repos.NonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	resp := createUserRequest(assert, "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb808s191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b", nonce.Address)
	assertErrorResponse(assert, resp)
}

func TestUserCreate_WrongAddress_Failure(t *testing.T) {
	assert := setupTest(t, 1)

	nonce := persist.UserNonce{
		Value:   "TestNonce",
		Address: persist.Address(strings.ToLower("0x9a3f9764B21adAF3C6fDf6f947e6D3349a3F8AC5")),
	}
	err := tc.repos.NonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	resp := createUserRequest(assert, "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b", nonce.Address)
	assertErrorResponse(assert, resp)
}

func TestUserCreate_NoNonce_Failure(t *testing.T) {
	assert := setupTest(t, 1)

	resp := createUserRequest(assert, "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b", "0x456d569592f15Af845D0dbe984C12BAB8F430e32")
	assertErrorResponse(assert, resp)
}

func TestUserLogin_Success(t *testing.T) {
	assert := setupTest(t, 1)

	user := persist.User{
		Addresses: []persist.Address{persist.Address(strings.ToLower("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5"))},
	}

	_, err := tc.repos.UserRepository.Create(context.Background(), user)
	assert.Nil(err)

	nonce := persist.UserNonce{
		Value:   "TestNonce",
		Address: user.Addresses[0],
	}
	err = tc.repos.NonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	resp := loginRequest(assert, "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b", "", nonce.Address, auth.WalletTypeEOA)
	assertValidResponse(assert, resp)

	type LoginOutput struct {
		auth.LoginOutput
		Error string `json:"error"`
	}
	output := &LoginOutput{}
	err = util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.Empty(output.Error)
	assert.True(output.SignatureValid)
	assert.NotEmpty(output.UserID)
}

func TestUserLoginGnosis_Success(t *testing.T) {
	assert := setupTest(t, 1)

	user := persist.User{
		Addresses: []persist.Address{persist.Address(strings.ToLower("0x60facEcd4dBF14f1ae647Afc3d1D071B1C29ACE4"))},
	}

	_, err := tc.repos.UserRepository.Create(context.Background(), user)
	assert.Nil(err)

	nonce := persist.UserNonce{
		Value:   "TEST NONCE",
		Address: user.Addresses[0],
	}
	err = tc.repos.NonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	resp := loginRequest(assert, "", auth.NoncePrepend+"TEST NONCE", nonce.Address, auth.WalletTypeGnosis)
	assertValidResponse(assert, resp)

	type LoginOutput struct {
		auth.LoginOutput
		Error string `json:"error"`
	}
	output := &LoginOutput{}
	err = util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.Empty(output.Error)
	assert.True(output.SignatureValid)
	assert.NotEmpty(output.UserID)
}

func TestUserLoginGnosis_WrongNonce_Failure(t *testing.T) {
	assert := setupTest(t, 1)

	user := persist.User{
		Addresses: []persist.Address{persist.Address(strings.ToLower("0x60facEcd4dBF14f1ae647Afc3d1D071B1C29ACE4"))},
	}

	_, err := tc.repos.UserRepository.Create(context.Background(), user)
	assert.Nil(err)

	nonce := persist.UserNonce{
		Value:   "TEST NONCE",
		Address: user.Addresses[0],
	}
	err = tc.repos.NonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	resp := loginRequest(assert, "", "0x", nonce.Address, auth.WalletTypeGnosis)
	assertErrorResponse(assert, resp)

	type LoginOutput struct {
		auth.LoginOutput
		Error string `json:"error"`
	}
	output := &LoginOutput{}
	err = util.UnmarshallBody(output, resp.Body)
	assert.False(output.SignatureValid)
	assert.Empty(output.UserID)
}

func TestUserLoginGnosis_WrongSig_Failure(t *testing.T) {
	assert := setupTest(t, 1)

	user := persist.User{
		Addresses: []persist.Address{persist.Address(strings.ToLower("0x60facEcd4dBF14f1ae647Afc3d1D071B1C29ACE4"))},
	}

	_, err := tc.repos.UserRepository.Create(context.Background(), user)
	assert.Nil(err)

	nonce := persist.UserNonce{
		Value:   " TEST NONCE",
		Address: user.Addresses[0],
	}
	err = tc.repos.NonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	resp := loginRequest(assert, "", " TEST NONCE", nonce.Address, auth.WalletTypeGnosis)
	assertErrorResponse(assert, resp)

	type LoginOutput struct {
		auth.LoginOutput
		Error string `json:"error"`
	}
	output := &LoginOutput{}
	err = util.UnmarshallBody(output, resp.Body)
	assert.False(output.SignatureValid)
	assert.Empty(output.UserID)
}
func TestUserLogin_WrongNonce_Failure(t *testing.T) {
	assert := setupTest(t, 1)

	user := persist.User{
		Addresses: []persist.Address{persist.Address(strings.ToLower("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5"))},
	}

	_, err := tc.repos.UserRepository.Create(context.Background(), user)
	assert.Nil(err)

	nonce := persist.UserNonce{
		Value:   "Wrong Nonce",
		Address: user.Addresses[0],
	}
	err = tc.repos.NonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	resp := loginRequest(assert, "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b", "", nonce.Address, auth.WalletTypeEOA)
	assertErrorResponse(assert, resp)
}

func TestUserLogin_WrongSig_Failure(t *testing.T) {
	assert := setupTest(t, 1)

	user := persist.User{
		Addresses: []persist.Address{persist.Address(strings.ToLower("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5"))},
	}

	_, err := tc.repos.UserRepository.Create(context.Background(), user)
	assert.Nil(err)

	nonce := persist.UserNonce{
		Value:   "TestNonce",
		Address: user.Addresses[0],
	}
	err = tc.repos.NonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	resp := loginRequest(assert, "0x0a22246c5feee38a80dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b", "", nonce.Address, auth.WalletTypeEOA)
	assertErrorResponse(assert, resp)
}

func TestUserLogin_WrongAddr_Failure(t *testing.T) {
	assert := setupTest(t, 1)

	user := persist.User{
		Addresses: []persist.Address{persist.Address(strings.ToLower("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5"))},
	}

	_, err := tc.repos.UserRepository.Create(context.Background(), user)
	assert.Nil(err)

	nonce := persist.UserNonce{
		Value:   "TestNonce",
		Address: user.Addresses[0],
	}
	err = tc.repos.NonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	resp := loginRequest(assert, "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b", "", "0xcb1b78568d0Ef81585f074b0Dfd6B743959070D9", auth.WalletTypeEOA)
	assertErrorResponse(assert, resp)
}

func TestUserLogin_NoNonce_Failure(t *testing.T) {
	assert := setupTest(t, 1)

	user := persist.User{
		Addresses: []persist.Address{persist.Address(strings.ToLower("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5"))},
	}

	_, err := tc.repos.UserRepository.Create(context.Background(), user)
	assert.Nil(err)

	resp := loginRequest(assert, "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b", "", "0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5", auth.WalletTypeEOA)
	assertErrorResponse(assert, resp)
}

func TestUserLogin_UserNotExist_Failure(t *testing.T) {
	assert := setupTest(t, 1)

	nonce := persist.UserNonce{
		Value:   "TestNonce",
		Address: persist.Address(strings.ToLower("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5")),
	}
	err := tc.repos.NonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	resp := loginRequest(assert, "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b", "", nonce.Address, auth.WalletTypeEOA)
	assertErrorResponse(assert, resp)
}

func TestUserLogin_UserNotOwnAddress_Failure(t *testing.T) {
	assert := setupTest(t, 1)

	nonce := persist.UserNonce{
		Value:   "TestNonce",
		Address: "0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5",
	}
	err := tc.repos.NonceRepository.Create(context.Background(), nonce)
	assert.Nil(err)

	resp := loginRequest(assert, "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b", "", nonce.Address, auth.WalletTypeEOA)
	assertErrorResponse(assert, resp)
}

func TestJwtValid_Success(t *testing.T) {
	assert := setupTest(t, 1)
	resp := jwtValidRequest(assert, tc.user1)
	assertValidJSONResponse(assert, resp)

	output := &auth.JWTValidateResponse{}
	err := util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.True(output.IsValid)
	assert.Equal(tc.user1.id, output.UserID)
}

func TestJwtValid_WrongSignatureAndClaims_Failure(t *testing.T) {
	assert := setupTest(t, 1)
	resp := jwtValidRequest(assert, generateTestUser(assert, tc.repos, "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJUZXN0IiwiaWF0IjoxNjMzMDE1MTM1LCJleHAiOjE2NjQ1NTExMzUsImF1ZCI6InRlc3QiLCJzdWIiOiJ0ZXN0IiwiVGVzdCI6IlRlc3QifQ.ewGO4x1xEN01CCZTp5vg0d_rxzdzH_rY0zBXVT1OVJY"))
	assertValidJSONResponse(assert, resp)

	output := &auth.JWTValidateResponse{}
	err := util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.False(output.IsValid)
}

func jwtValidRequest(assert *assert.Assertions, t *TestUser) *http.Response {
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/auth/jwt_valid", tc.serverURL),
		nil)
	assert.Nil(err)

	resp, err := t.client.Do(req)
	assert.Nil(err)
	return resp
}

func getPreflightRequest(assert *assert.Assertions, t *TestUser) *http.Response {
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/auth/get_preflight?address=%s", tc.serverURL, t.address),
		nil)
	assert.Nil(err)
	resp, err := t.client.Do(req)
	assert.Nil(err)
	return resp
}

func createUserRequest(assert *assert.Assertions, sig string, address persist.Address) *http.Response {
	body := map[string]interface{}{"address": address, "signature": sig}
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

func loginRequest(assert *assert.Assertions, sig, nonce string, address persist.Address, wt auth.WalletType) *http.Response {
	body := map[string]interface{}{"address": address, "wallet_type": wt}
	if nonce != "" {
		body["nonce"] = nonce
	}
	if sig != "" {
		body["signature"] = sig
	}
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
