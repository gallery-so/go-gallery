package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/runtime"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// Should be called at the beginning of every integration test
// Initializes the runtime, connects to mongodb, and starts a test server
func setup() (*httptest.Server, string, *runtime.Runtime) {
	// Initialize runtime
	runtime, _ := runtime.RuntimeGet(runtime.ConfigLoad())

	// Initialize test server
	gin.SetMode(gin.ReleaseMode) // Prevent excessive logs
	runtime.Router = gin.Default()
	ts := httptest.NewServer(HandlersInit(runtime))
	log.Info("server connected! ✅")

	return ts, fmt.Sprintf("%s/glry/v1", ts.URL), runtime
}

// Should be called at the end of every integration test
func teardown(ts *httptest.Server) {
	log.Info("tearing down test suite...")
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