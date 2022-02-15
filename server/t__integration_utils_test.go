package server

import (
	"bytes"
	"crypto/ecdsa"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"

	"github.com/asottile/dockerfile"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/user"
	"github.com/mikeydub/go-gallery/util"
	"github.com/ory/dockertest"
	"github.com/ory/dockertest/docker"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v2"
)

type TestAddressFile struct {
	Wallet1     string `json:"wallet_1"`
	PrivateKey1 string `json:"pk_1"`
}

// N.B. This isn't the entire Docker Compose spec...
type ComposeFile struct {
	Version  string
	Services map[string]Service
}

type Service struct {
	Image       string
	Ports       []string
	Build       map[string]string
	Environment []string
}

func configureContainerCleanup(config *docker.HostConfig) {
	config.AutoRemove = true
	config.RestartPolicy = docker.RestartPolicy{Name: "no"}
}

func waitOnDB() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("db is not available")
		}
	}()
	postgres.NewClient().Close()
	return
}

func waitOnCache() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("cache is not available")
		}
	}()
	redis.NewCache(0).Close(false)
	return
}

func loadComposeFile(path string) (f ComposeFile) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}

	err = yaml.Unmarshal(data, &f)
	if err != nil {
		log.Fatal(err)
	}

	return
}

func getImageAndVersion(s string) ([]string, error) {
	imgAndVer := strings.Split(s, ":")
	if len(imgAndVer) != 2 {
		return nil, errors.New("no version specified for image")
	}
	return imgAndVer, nil
}

func getBuildImage(s Service) ([]string, error) {
	res, err := dockerfile.ParseFile(".." + string(filepath.Separator) + s.Build["dockerfile"])
	if err != nil {
		log.Fatal(err)
	}

	for _, cmd := range res {
		if cmd.Cmd == "FROM" {
			return getImageAndVersion(cmd.Value[0])
		}
	}

	return nil, errors.New("no `FROM` directive found in dockerfile")
}

func initPostgres(pool *dockertest.Pool) (*dockertest.Resource, *sql.DB) {
	apps := loadComposeFile("../docker-compose.yml")
	imgAndVer, err := getBuildImage(apps.Services["postgres"])
	if err != nil {
		log.Fatal(err)
	}

	pg, err := pool.RunWithOptions(
		&dockertest.RunOptions{
			Repository: imgAndVer[0],
			Tag:        imgAndVer[1],
			Env:        apps.Services["postgres"].Environment,
		}, configureContainerCleanup,
	)
	if err != nil {
		log.Fatalf("could not start postgres: %s", err)
	}

	// Patch environment to use container
	hostAndPort := strings.Split(pg.GetHostPort("5432/tcp"), ":")
	viper.SetDefault("POSTGRES_HOST", hostAndPort[0])
	viper.SetDefault("POSTGRES_PORT", hostAndPort[1])

	if err = pool.Retry(waitOnDB); err != nil {
		log.Fatalf("could not connect to postgres: %s", err)
	}

	// Seed db
	db = postgres.NewClient()
	for _, f := range []string{"../scripts/initial_setup.sql", "../scripts/post_import.sql"} {
		migration, err := os.ReadFile(f)
		if err != nil {
			panic(err)
		}

		_, err = db.Exec(string(migration))
		if err != nil {
			log.Fatalf("failed to seed the db: %s", err)
		}
	}

	return pg, db
}

func initRedis(pool *dockertest.Pool) *dockertest.Resource {
	apps := loadComposeFile("../docker-compose.yml")
	imgAndVer, err := getImageAndVersion(apps.Services["redis"].Image)
	if err != nil {
		log.Fatal(err)
	}

	rd, err := pool.RunWithOptions(
		&dockertest.RunOptions{
			Repository: imgAndVer[0],
			Tag:        imgAndVer[1],
		}, configureContainerCleanup,
	)
	if err != nil {
		log.Fatalf("could not start redis: %s", err)
	}

	// Patch environment to use container
	viper.SetDefault("REDIS_URL", rd.GetHostPort("6379/tcp"))
	if err = pool.Retry(waitOnCache); err != nil {
		log.Fatalf("could not connect to redis: %s", err)
	}

	return rd
}

func loadWallet(f string) *TestWallet {
	dat, err := os.ReadFile(f)
	if err != nil {
		log.Fatalf("could not load wallet file: %s", err)
	}

	var wallets TestAddressFile
	err = json.Unmarshal(dat, &wallets)
	if err != nil {
		log.Fatalf("wallet file is an unexpected format: %s", err)
	}

	pk, err := crypto.HexToECDSA(wallets.PrivateKey1)
	if err != nil {
		log.Fatalf("private key is malformed: %s", err)
	}

	return &TestWallet{pk, persist.Address(wallets.Wallet1)}
}

func newClient() *http.Client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatalf("could not create cookie jar: %s", err)
	}
	return &http.Client{Jar: jar}
}

func getCookieByName(n string, cookies []*http.Cookie) *http.Cookie {
	for _, c := range cookies {
		if c.Name == n {
			return c
		}
	}
	return nil
}

func fetchUser(s suite.Suite, serverURL string, userID persist.DBID) *user.GetUserOutput {
	resp, err := http.Get(fmt.Sprintf("%s/users/get?user_id=%s", serverURL, userID))
	s.NoError(err)
	defer resp.Body.Close()

	assertValidResponse(s.Assertions, resp)

	output := &user.GetUserOutput{}
	err = util.UnmarshallBody(output, resp.Body)
	s.NoError(err)

	return output
}

func fetchCurrentUserResponse(s suite.Suite, serverURL string, client *http.Client, jwt string) *http.Response {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/users/get/current", serverURL), nil)
	s.NoError(err)

	req.AddCookie(&http.Cookie{Name: auth.JWTCookieKey, Value: jwt})
	resp, err := client.Do(req)
	s.NoError(err)

	return resp
}

func fetchCurrentUserIsValid(s suite.Suite, serverURL string, client *http.Client, jwt string) *user.GetUserOutput {
	resp := fetchCurrentUserResponse(s, serverURL, client, jwt)

	assertValidResponse(s.Assertions, resp)

	output := &user.GetUserOutput{}
	err := util.UnmarshallBody(output, resp.Body)
	s.NoError(err)

	return output
}

func fetchNonce(s suite.Suite, serverURL string, address persist.Address) string {
	resp, err := http.Get(
		fmt.Sprintf("%s/auth/get_preflight?address=%s", serverURL, address),
	)
	s.NoError(err)
	defer resp.Body.Close()

	assertValidResponse(s.Assertions, resp)

	type PreflightResp struct {
		auth.GetPreflightOutput
		Error string `json:"error"`
	}
	output := &PreflightResp{}
	err = util.UnmarshallBody(output, resp.Body)
	s.NoError(err)
	s.Empty(output.Error)

	return output.Nonce
}

func signNonce(s suite.Suite, n string, pk *ecdsa.PrivateKey) []byte {
	hash := crypto.Keccak256Hash([]byte(n))
	sig, err := crypto.Sign(hash.Bytes(), pk)
	s.NoError(err)

	return sig
}

func createNewUser(s suite.Suite, serverURL, nonce string, account *TestWallet) *user.CreateUserOutput {
	sig := signNonce(s, nonce, account.pk)
	data, err := json.Marshal(map[string]interface{}{
		"address":     account.address,
		"nonce":       nonce,
		"wallet_type": 0,
		"signature":   "0x" + hex.EncodeToString(sig),
	})
	s.NoError(err)

	resp, err := http.Post(
		fmt.Sprintf("%s/users/create", serverURL),
		"application/json",
		bytes.NewBuffer(data),
	)
	s.NoError(err, "failed to create user")
	defer resp.Body.Close()

	assertValidResponse(s.Assertions, resp)

	output := &user.CreateUserOutput{}
	err = util.UnmarshallBody(output, resp.Body)
	s.NoError(err)

	return output
}

func loginUser(s suite.Suite, serverURL, nonce string, wallet *TestWallet) (*auth.LoginOutput, *http.Cookie) {
	sig := signNonce(s, nonce, wallet.pk)
	data, err := json.Marshal(map[string]interface{}{
		"address":     wallet.address,
		"nonce":       nonce,
		"wallet_type": 0,
		"signature":   "0x" + hex.EncodeToString(sig),
	})
	s.NoError(err)

	resp, err := http.Post(
		fmt.Sprintf("%s/users/login", serverURL),
		"application/json",
		bytes.NewBuffer(data),
	)
	s.NoError(err, "failed to login user")
	defer resp.Body.Close()

	assertValidResponse(s.Assertions, resp)

	output := &auth.LoginOutput{}
	err = util.UnmarshallBody(output, resp.Body)
	s.NoError(err)

	jwtCookie := getCookieByName(auth.JWTCookieKey, resp.Cookies())
	s.NotEmpty(jwtCookie)

	return output, jwtCookie
}

func logoutUser(s suite.Suite, serverURL string, client *http.Client) *http.Response {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/auth/logout", serverURL), nil)
	s.NoError(err)

	resp, err := client.Do(req)
	s.NoError(err)

	assertValidResponse(s.Assertions, resp)

	return resp
}
