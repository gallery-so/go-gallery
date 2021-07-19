package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/mikeydub/go-gallery/runtime"
	"github.com/stretchr/testify/assert"
)

func TestHealthcheck(t *testing.T) {
	assert := assert.New(t)

	resp, err := http.Get(fmt.Sprintf("%s/health", serverUrl))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := HealthcheckResponse{}
	runtime.UnmarshalBody(&body, resp.Body, r)
	assert.Equal("gallery operational", body.Message)
	assert.Equal("local", body.Env)
}
