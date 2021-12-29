package server

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/mongo"
)

type TestConfig struct {
	server              *httptest.Server
	serverURL           string
	repos               *repositories
	mgoClient           *mongo.Client
	db                  *sql.DB
	user1               *TestUser
	user2               *TestUser
	openseaCache        memstore.Cache
	unassignedCache     memstore.Cache
	galleriesCache      memstore.Cache
	galleriesCacheToken memstore.Cache
}

var tc *TestConfig

type TestUser struct {
	id       persist.DBID
	address  persist.Address
	jwt      string
	username string
}

func generateTestUser(repos *repositories, username string) *TestUser {
	ctx := context.Background()

	address := persist.Address(strings.ToLower(fmt.Sprintf("0x%s", util.RandStringBytes(40))))
	user := persist.User{
		Username:           username,
		UsernameIdempotent: strings.ToLower(username),
		Addresses:          []persist.Address{address},
	}
	id, err := repos.userRepository.Create(ctx, user)
	if err != nil {
		log.Fatal(err)
	}
	jwt, err := middleware.JWTGeneratePipeline(ctx, id)
	if err != nil {
		log.Fatal(err)
	}
	auth.NonceRotate(ctx, address, id, repos.nonceRepository)
	log.Info(id, username)
	return &TestUser{id, address, jwt, username}
}

// Should be called at the beginning of every integration test
// Initializes the runtime, connects to mongodb, and starts a test server
func initializeTestEnv(v int) *TestConfig {
	gin.SetMode(gin.ReleaseMode) // Prevent excessive logs
	ts := httptest.NewServer(CoreInit())

	mclient := newMongoClient()
	repos := newRepos()
	opensea, unassigned, galleries, galleriesToken := redis.NewCache(0), redis.NewCache(1), redis.NewCache(2), redis.NewCache(3)
	log.Info("test server connected! ✅")
	return &TestConfig{
		server:              ts,
		serverURL:           fmt.Sprintf("%s/glry/v%d", ts.URL, v),
		repos:               repos,
		mgoClient:           mclient,
		db:                  postgres.NewClient(),
		user1:               generateTestUser(repos, "bob"),
		user2:               generateTestUser(repos, "john"),
		openseaCache:        opensea,
		unassignedCache:     unassigned,
		galleriesCache:      galleries,
		galleriesCacheToken: galleriesToken,
	}
}

// Should be called at the end of every integration test
func teardown() {
	log.Info("tearing down test suite...")
	tc.server.Close()
	clearDB()
}

func clearDB() {
	tc.mgoClient.Database("gallery").Drop(context.Background())
	defer tc.db.Close()
	dropSQL := `TRUNCATE users, nfts, collections, galleries, tokens, contracts, membership, access, nonces, login_attempts, access, backups;`
	_, err := tc.db.Exec(dropSQL)
	if err != nil {
		panic(err)
	}
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

func assertErrorResponse(assert *assert.Assertions, resp *http.Response) {
	assert.NotEqual(http.StatusOK, resp.StatusCode, "Status should not be 200")
	val, ok := resp.Header["Content-Type"]
	assert.True(ok, "Content-Type header should be set")
	assert.Equal("application/json; charset=utf-8", val[0], "Response should be in JSON")
}

func setupTest(t *testing.T, v int) *assert.Assertions {
	tc = initializeTestEnv(v)
	t.Cleanup(teardown)
	return assert.New(t)
}
