package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/mikeydub/go-gallery/runtime"
)

func TestHealthcheck(t *testing.T) {
	assert, testServer, serverUrl, r := setup(t)
	defer teardown(testServer)

	resp, err := http.Get(fmt.Sprintf("%s/health", serverUrl))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := HealthcheckResponse{}
	runtime.UnmarshalBody(&body, resp.Body, r)

	assert.Equal(body.Message, "gallery operational")
	assert.Equal(body.Env, "local")
}
