package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var db *sql.DB

type TestConfig struct {
	server              *httptest.Server
	serverURL           string
	repos               *persist.Repositories
	user1               *TestUser
	user2               *TestUser
	openseaCache        memstore.Cache
	unassignedCache     memstore.Cache
	galleriesCache      memstore.Cache
	galleriesCacheToken memstore.Cache
}

var tc *TestConfig
var ts *httptest.Server

type TestWallet struct {
	pk      *ecdsa.PrivateKey
	address persist.Address
}

type TestUser struct {
	*TestWallet
	id       persist.DBID
	jwt      string
	username string
	client   *http.Client
}

func generateTestUser(a *assert.Assertions, repos *persist.Repositories, jwt string) *TestUser {
	ctx := context.Background()

	username := util.RandStringBytes(40)

	pk, err := crypto.GenerateKey()
	a.NoError(err)

	pub := crypto.PubkeyToAddress(pk.PublicKey).String()
	wallet := TestWallet{pk, persist.Address(strings.ToLower(pub))}

	user := persist.User{
		Username:           persist.NullString(username),
		UsernameIdempotent: persist.NullString(strings.ToLower(username)),
		Addresses:          []persist.Address{wallet.address},
	}
	id, err := repos.UserRepository.Create(ctx, user)
	a.NoError(err)
	j, err := cookiejar.New(nil)
	a.NoError(err)
	c := &http.Client{Jar: j}

	if jwt == "" {
		jwt, err = auth.JWTGeneratePipeline(ctx, id)
		a.NoError(err)
	}

	getFakeCookie(a, jwt, c)
	auth.NonceRotate(ctx, wallet.address, id, repos.NonceRepository)
	log.Info(id, username)
	return &TestUser{&wallet, id, jwt, username, c}
}

// Should be called at the beginning of every integration test
// Initializes the runtime and starts a test server
func initializeTestEnv(a *assert.Assertions, v int) *TestConfig {
	setDefaults()

	if db == nil {
		db = postgres.NewClient()
	}

	return initializeTestServer(db, a, v)
}

func initializeTestServer(db *sql.DB, a *assert.Assertions, v int) *TestConfig {
	router := CoreInit(db)
	router.POST("/fake-cookie", fakeCookie)
	ts = httptest.NewServer(router)

	gin.SetMode(gin.ReleaseMode) // Prevent excessive logs
	repos := newRepos(db)
	galleries, galleriesToken := redis.NewCache(0), redis.NewCache(1)
	log.Infof("test server connected at %s ✅", ts.URL)

	return &TestConfig{
		server:    ts,
		serverURL: fmt.Sprintf("%s/glry/v%d", ts.URL, v),
		repos:     repos,
		user1:     generateTestUser(a, repos, ""),
		user2:     generateTestUser(a, repos, ""),

		galleriesCache:      galleries,
		galleriesCacheToken: galleriesToken,
	}
}

// Should be called at the end of every integration test
func teardown() {
	log.Info("tearing down test suite...")
	tc.server.Close()
	clearDB()
	tc.galleriesCache.Close(true)
	tc.galleriesCacheToken.Close(true)
}

func clearDB() {
	dropSQL := `TRUNCATE users, nfts, collections, galleries, tokens, contracts, membership, access, nonces, login_attempts, access, backups;`
	_, err := db.Exec(dropSQL)
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
}

func setupTest(t *testing.T, v int) *assert.Assertions {
	a := assert.New(t)
	tc = initializeTestEnv(a, v)
	t.Cleanup(teardown)
	return assert.New(t)
}

type fakeCookieInput struct {
	JWT string `json:"jwt"`
}

func fakeCookie(ctx *gin.Context) {
	var input fakeCookieInput
	err := ctx.ShouldBindJSON(&input)
	if err != nil {
		ctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	ctx.SetCookie(auth.JWTCookieKey, input.JWT, 3600, "/", "", false, true)
	ctx.Status(http.StatusOK)
}

func getFakeCookie(a *assert.Assertions, pJWT string, c *http.Client) {
	var input fakeCookieInput
	input.JWT = pJWT
	buf := new(bytes.Buffer)
	err := json.NewEncoder(buf).Encode(input)
	a.NoError(err)
	resp, err := c.Post(fmt.Sprintf("%s/fake-cookie", ts.URL), "application/json", buf)
	a.NoError(err)
	assertValidResponse(a, resp)

}
