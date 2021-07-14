package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_lib"
	"github.com/stretchr/testify/assert"
)

type serverUrl = string

// Should be called at the beginning of every integration test
// Initializes the runtime, connects to mongodb, and starts a test server
func setup(t *testing.T) (*assert.Assertions,
	*httptest.Server,
	serverUrl,
	*glry_core.Runtime) {
	// Initialize assertion handler to be used in actual test downstream
	assert := assert.New(t)

	// Initialize runtime
	runtime, err := glry_core.RuntimeGet(
		&glry_core.GLRYconfig{
			// TODO: get these from ENV
			MongoURLstr: "mongodb://127.0.0.1:27017",
			MongoDBnameStr: "gallery",
			Port: 4000,
			BaseURL: "http://localhost:4000",
			EnvStr: "glry_test",
		})
	assert.Nil(err)

	// Initialize test server
	gin.SetMode(gin.ReleaseMode) // Prevent excessive logs
	runtime.Router = gin.Default()
	ts := httptest.NewServer(glry_lib.HandlersInit(runtime))

	return assert, ts, fmt.Sprintf("%s/glry/v1", ts.URL), runtime
}

// Should be called at the end of every integration test
func teardown(ts *httptest.Server) {
	ts.Close()
	// TODO: drop mongo database, etc.
}

func assertValidJSONResponse(assert *assert.Assertions, resp *http.Response) {
	assert.Equal(resp.StatusCode, http.StatusOK, "Status should be 200")
	val, ok := resp.Header["Content-Type"]
	assert.True(ok, "Content-Type header should be set")
	assert.Equal(val[0], "application/json; charset=utf-8", "Response should be in JSON")
}