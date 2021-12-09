package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestAuthVerifySignature_Success(t *testing.T) {
	assert := setupTest(t, 1)

	client, err := ethclient.Dial(viper.GetString("CONTRACT_INTERACTION_URL"))
	if err != nil {
		panic(err)
	}

	testNonce := "TestNonce"
	sig := "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b"
	addr := persist.Address("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5")

	success, err := authVerifySignatureAllMethods(sig, testNonce, addr, walletTypeEOA, client)
	assert.Nil(err)
	assert.True(success)
}

func TestAuthVerifySignature_WrongNonce_Failure(t *testing.T) {
	assert := setupTest(t, 1)

	client, err := ethclient.Dial(viper.GetString("CONTRACT_INTERACTION_URL"))
	if err != nil {
		panic(err)
	}
	testNonce := "Wrong Nonce despite address signing sig"
	sig := "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b"
	addr := persist.Address("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5")

	success, err := authVerifySignatureAllMethods(sig, testNonce, addr, walletTypeEOA, client)
	assert.NotNil(err)
	assert.False(success)
}

func TestAuthVerifySignature_WrongAddress_Failure(t *testing.T) {
	assert := setupTest(t, 1)

	client, err := ethclient.Dial(viper.GetString("CONTRACT_INTERACTION_URL"))
	if err != nil {
		panic(err)
	}

	testNonce := "TestNonce"
	sig := "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b"
	addr := persist.Address("0x456d569592f15Af845D0dbe984C12BAB8F430e32")

	success, err := authVerifySignatureAllMethods(sig, testNonce, addr, walletTypeEOA, client)
	assert.NotNil(err)
	assert.False(success)
}

func TestJwtValid_Success(t *testing.T) {
	assert := setupTest(t, 1)
	resp := jwtValidRequest(assert, tc.user1.jwt)
	assertValidJSONResponse(assert, resp)

	output := &middleware.JWTValidateResponse{}
	err := util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.True(output.IsValid)
	assert.Equal(tc.user1.id, output.UserID)
}

func TestJwtValid_WrongSignatureAndClaims_Failure(t *testing.T) {
	assert := setupTest(t, 1)
	resp := jwtValidRequest(assert, "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJUZXN0IiwiaWF0IjoxNjMzMDE1MTM1LCJleHAiOjE2NjQ1NTExMzUsImF1ZCI6InRlc3QiLCJzdWIiOiJ0ZXN0IiwiVGVzdCI6IlRlc3QifQ.ewGO4x1xEN01CCZTp5vg0d_rxzdzH_rY0zBXVT1OVJY")
	assertValidJSONResponse(assert, resp)

	output := &middleware.JWTValidateResponse{}
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
