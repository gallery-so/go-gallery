package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/assert"
)

func TestAuthVerifySignature_Success(t *testing.T) {
	setupTest(t)

	testNonce := "TestNonce"
	sig := "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b"
	addr := "0x456d569592f15Af845D0dbe984C12BAB8F430e31"

	success, err := authVerifySignatureAllMethods(sig, testNonce, addr, tc.r)
	assert.Nil(t, err)
	assert.True(t, success)
}

func TestAuthVerifySignature_WrongNonce_Failure(t *testing.T) {
	setupTest(t)

	testNonce := "Wrong Nonce despite address signing sig"
	sig := "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b"
	addr := "0x456d569592f15Af845D0dbe984C12BAB8F430e31"

	success, err := authVerifySignatureAllMethods(sig, testNonce, addr, tc.r)
	assert.NotNil(t, err)
	assert.False(t, success)
}

func TestAuthVerifySignature_WrongAddress_Failure(t *testing.T) {
	setupTest(t)

	testNonce := "TestNonce"
	sig := "0x0a22246c5feee38a90dc6898b453c944e7e7c2f9850218d7c13f3f17f992ea691bb8083191a59ad2c83a5d7f4b41d85df1e693a96b5a251f0a66751b7dc235091b"
	addr := "0x456d569592f15Af845D0dbe984C12BAB8F430e32"

	success, err := authVerifySignatureAllMethods(sig, testNonce, addr, tc.r)
	assert.NotNil(t, err)
	assert.False(t, success)
}

func TestJwtValid_Success(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)
	resp := jwtValidRequest(assert, tc.user1.jwt)
	assertValidJSONResponse(assert, resp)

	output := &jwtValidateResponse{}
	err := util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.True(output.IsValid)
	assert.Equal(tc.user1.id, output.UserID)
}

func TestJwtValid_WrongSignatureAndClaims_Failure(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)
	resp := jwtValidRequest(assert, "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJUZXN0IiwiaWF0IjoxNjMzMDE1MTM1LCJleHAiOjE2NjQ1NTExMzUsImF1ZCI6InRlc3QiLCJzdWIiOiJ0ZXN0IiwiVGVzdCI6IlRlc3QifQ.ewGO4x1xEN01CCZTp5vg0d_rxzdzH_rY0zBXVT1OVJY")
	assertValidJSONResponse(assert, resp)

	output := &jwtValidateResponse{}
	err := util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.False(output.IsValid)
}

func jwtValidRequest(assert *assert.Assertions, jwt string) *http.Response {
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/auth/jwt_valid", tc.serverURL),
		nil)
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	return resp
}
