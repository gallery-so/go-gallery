package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/mikeydub/go-gallery/util"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

type TestUser struct {
	id       persist.DBID
	address  string
	jwt      string
	username string
}

func generateTestUser(r *runtime.Runtime) *TestUser {
	ctx := context.Background()
	username := util.RandStringBytes(8)
	address := fmt.Sprintf("0x%s", util.RandStringBytes(40))
	user := &persist.User{
		UserName: username,
		Addresses: []string{address},
	}
	id, _ := persist.UserCreate(ctx, user, r)
	jwt, _ := jwtGeneratePipeline(ctx, id, r)

	return &TestUser{id, address, jwt, username}
}

// Should be called at the beginning of every integration test
// Initializes the runtime, connects to mongodb, and starts a test server
func setup() *TestConfig {
	// Initialize runtime
	runtime, _ := runtime.GetRuntime(runtime.ConfigLoad())

	// Initialize test server
	gin.SetMode(gin.ReleaseMode) // Prevent excessive logs
	runtime.Router = gin.Default()
	ts := httptest.NewServer(CoreInit(runtime))

	log.Info("server connected! ✅")

	return &TestConfig{
		server:    ts,
		serverURL: fmt.Sprintf("%s/glry/v1", ts.URL),
		r:         runtime,
		user1:     generateTestUser(runtime),
		user2:     generateTestUser(runtime),
	}
}

// Should be called at the end of every integration test
func teardown(ts *httptest.Server) {
	log.Info("tearing down test suite...")
	ts.Close()
	tc.r.DB.MongoDB.Drop(context.Background())
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
