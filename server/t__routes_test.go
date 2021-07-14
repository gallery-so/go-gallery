package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_lib"
)

func TestHealthcheck(t *testing.T) {
	assert, testServer, serverUrl, runtime := setup(t)
	defer teardown(testServer)

	resp, err := http.Get(fmt.Sprintf("%s/health", serverUrl))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := glry_lib.HealthcheckResponse{}
	glry_core.UnmarshalBody(&body, resp.Body, runtime)

	assert.Equal(body.Message, "gallery operational")
	assert.Equal(body.Env, "local")
}