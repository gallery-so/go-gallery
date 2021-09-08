package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/mikeydub/go-gallery/runtime"
	"github.com/stretchr/testify/assert"
)

func TestAuthPreflightUserExists_Success(t *testing.T) {
	t.Cleanup(clearDB)
	assert := assert.New(t)

	resp := getPreflightRequest(assert, tc.user1.address, tc.user1.jwt)
	assertValidResponse(assert, resp)

	type PreflightResp struct {
		authUserGetPreflightOutput
		Error string `json:"error"`
	}
	output := &PreflightResp{}
	err := runtime.UnmarshallBody(output, resp.Body, tc.r)
	assert.Nil(err)
	assert.Empty(output.Error)
	assert.True(output.UserExists)
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
