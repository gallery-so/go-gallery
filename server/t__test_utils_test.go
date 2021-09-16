package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/mikeydub/go-gallery/util"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

type TestConfig struct {
	server    *httptest.Server
	serverURL string
	r         *runtime.Runtime
	user1     *TestUser
	user2     *TestUser
}

var tc *TestConfig

type TestUser struct {
	id       persist.DBID
	address  string
	jwt      string
	username string
}

func generateTestUser(r *runtime.Runtime, username string) *TestUser {
	ctx := context.Background()

	address := fmt.Sprintf("0x%s", util.RandStringBytes(40))
	user := &persist.User{
		UserName:           username,
		UserNameIdempotent: strings.ToLower(username),
		Addresses:          []string{strings.ToLower(address)},
	}
	id, err := persist.UserCreate(ctx, user, r)
	if err != nil {
		log.Fatal(err)
	}
	jwt, err := jwtGeneratePipeline(ctx, id, r)
	if err != nil {
		log.Fatal(err)
	}
	authNonceRotateDb(ctx, address, id, r)
	log.Info(id, username)
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

	log.Info("test server connected! ✅")

	return &TestConfig{
		server:    ts,
		serverURL: fmt.Sprintf("%s/glry/v1", ts.URL),
		r:         runtime,
		user1:     generateTestUser(runtime, "bob"),
		user2:     generateTestUser(runtime, "john"),
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
