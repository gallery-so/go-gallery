package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/stretchr/testify/assert"
)

type serverUrl = string

// Should be called at the beginning of every integration test
// Initializes the runtime, connects to mongodb, and starts a test server
func setup(t *testing.T) (*assert.Assertions,
	*httptest.Server,
	serverUrl,
	*runtime.Runtime) {
	// Initialize assertion handler to be used in actual test downstream
	assert := assert.New(t)

	// Initialize runtime
	runtime, err := runtime.RuntimeGet(runtime.ConfigLoad())
	assert.Nil(err)

	// Initialize test server
	gin.SetMode(gin.ReleaseMode) // Prevent excessive logs
	runtime.Router = gin.Default()
	ts := httptest.NewServer(HandlersInit(runtime))

	return assert, ts, fmt.Sprintf("%s/glry/v1", ts.URL), runtime
}

// Should be called at the end of every integration test
func teardown(ts *httptest.Server) {
	ts.Close()
	// TODO: drop mongo database, etc.
}

func assertValidJSONResponse(assert *assert.Assertions, resp *http.Response) {
	assert.Equal(http.StatusOK, resp.StatusCode, "Status should be 200")
	val, ok := resp.Header["Content-Type"]
	assert.True(ok, "Content-Type header should be set")
	assert.Equal("application/json; charset=utf-8", val[0], "Response should be in JSON")
}

func assertGalleryErrorResponse(assert *assert.Assertions, resp *http.Response) {
	assert.NotEqual(http.StatusOK, resp.StatusCode, "Status should not be 200")
	val, ok := resp.Header["Content-Type"]
	assert.True(ok, "Content-Type header should be set")
	assert.Equal("application/json; charset=utf-8", val[0], "Response should be in JSON")
}