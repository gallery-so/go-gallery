package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/assert"
)

func TestHealthcheck(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	resp, err := http.Get(fmt.Sprintf("%s/health", tc.serverURL))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := healthcheckResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal("gallery operational", body.Message)
	assert.Equal("local", body.Env)
}
