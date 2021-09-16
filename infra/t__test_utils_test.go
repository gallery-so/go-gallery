package infra

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/runtime"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

type TestConfig struct {
	server    *httptest.Server
	serverURL string
	r         *runtime.Runtime
}

var tc *TestConfig

// Should be called at the beginning of every integration test
// Initializes the runtime, connects to mongodb, and starts a test server
func setup() *TestConfig {
	// Initialize runtime
	runtime, _ := runtime.GetRuntime(runtime.ConfigLoad())

	// Initialize test server
	gin.SetMode(gin.ReleaseMode) // Prevent excessive logs
	runtime.Router = gin.Default()
	ts := httptest.NewServer(CoreInit(runtime))

	log.Info("test server connected! ✅")

	return &TestConfig{
		server:    ts,
		serverURL: fmt.Sprintf("%s/infra/v1", ts.URL),
		r:         runtime,
	}
}

// Should be called at the end of every integration test
func teardown() {
	log.Info("tearing down test suite...")
	tc.server.Close()
	clearDB()
}

func clearDB() {
	tc.r.DB.MongoClient.Database(runtime.GalleryDBName).Drop(context.Background())
}

func assertValidResponse(assert *assert.Assertions, resp *http.Response) {

	assert.Equal(http.StatusOK, resp.StatusCode, "Status should be 200")
}

func assertValidJSONResponse(assert *assert.Assertions, resp *http.Response) {
	assertValidResponse(assert, resp)
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

func setupTest(t *testing.T) {
	tc = setup()
	t.Cleanup(teardown)
}
