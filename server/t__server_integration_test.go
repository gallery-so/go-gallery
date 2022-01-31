package server

import (
	"bytes"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/util"
	"github.com/ory/dockertest"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
)

type UserTestSuite struct {
	suite.Suite
	tc            *TestConfig
	version       int
	pool          *dockertest.Pool
	pgResource    *dockertest.Resource
	redisResource *dockertest.Resource
	db            *sql.DB
}

func (s *UserTestSuite) SetupTest() {
	setDefaults()

	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("could not connect to docker: %s", err)
	}
	pg, pgClient := initPostgres(pool)
	rd := initRedis(pool)

	s.version = 1
	s.pool = pool
	s.db = pgClient
	s.pgResource = pg
	s.redisResource = rd
	s.tc = initializeTestServer(s.db, s.Assertions, s.version)
}

func (s *UserTestSuite) TearDownTest() {
	// Kill containers
	for _, r := range []*dockertest.Resource{s.pgResource, s.redisResource} {
		if err := s.pool.Purge(r); err != nil {
			log.Fatalf("could not purge resource: %s", err)
		}
	}

	s.db.Close()
	s.tc.server.Close()
}

func (s *UserTestSuite) TestUserCanLogin() {
	nonce := s.fetchNonce()
	resp, err := s.loginUser(nonce)

	s.NoError(err)
	defer resp.Body.Close()

	assertValidResponse(s.Assertions, resp)
}

func (s *UserTestSuite) fetchNonce() string {
	resp, err := http.Get(
		fmt.Sprintf("%s/auth/get_preflight?address=%s", s.tc.serverURL, s.tc.user1.address),
	)
	s.NoError(err)
	defer resp.Body.Close()

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

func (s *UserTestSuite) signNonce(n string) []byte {
	hash := crypto.Keccak256Hash([]byte(n))
	sig, err := crypto.Sign(hash.Bytes(), s.tc.user1.pk)
	s.NoError(err)

	return sig
}

func (s *UserTestSuite) loginUser(nonce string) (*http.Response, error) {
	sig := s.signNonce(nonce)
	data, err := json.Marshal(map[string]interface{}{
		"address":     s.tc.user1.address,
		"nonce":       nonce,
		"wallet_type": 0,
		"signature":   "0x" + hex.EncodeToString(sig),
	})
	s.NoError(err)

	return http.Post(
		fmt.Sprintf("%s/users/login", s.tc.serverURL),
		"application/json",
		bytes.NewBuffer(data),
	)
}

func TestUserTestSuite(t *testing.T) {
	suite.Run(t, new(UserTestSuite))
}
