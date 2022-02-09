package server

import (
	"database/sql"
	"errors"
	"flag"
	"testing"

	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/ory/dockertest"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
)

const (
	// networks
	ethMainnet = iota + 1
	_
	ethRopsten
	ethRinkeby

	// eligible contracts
	contractAddressesEthMainnet = "0xe01569ca9b39E55Bc7C0dFa09F05fa15CB4C7698=[0,1,2,3,4,5,6,7,8]"
	contractAddressesEthRinkeby = "0x93eC9b03a9C14a530F582aef24a21d7FC88aaC46=[0,1,2,3,4,5,6,7,8]"

	// node providers
	contractInteractionURLEthMainnet = "https://eth-mainnet.alchemyapi.io/v2/_2u--i79yarLYdOT4Bgydqa0dBceVRLD"
	contractInteractionURLEthRinkeby = "https://eth-rinkeby.alchemyapi.io/v2/_2u--i79yarLYdOT4Bgydqa0dBceVRLD"

	// blockchains
	blockchainEth = "ethereum"

	// live wallets
	testWalletFileEthMainnet = "../test-wallet.json"
)

var (
	blockchain = flag.String("chain", blockchainEth, "blockchain to run against")
	chainID    = flag.Int("chainID", ethMainnet, "chainID to run against")
	walletFile = flag.String("walletFile", testWalletFileEthMainnet, "walletFile to load")
)

type IntegrationTestConfig struct {
	*TestConfig
	pool          *dockertest.Pool
	pgResource    *dockertest.Resource
	redisResource *dockertest.Resource
	db            *sql.DB
}

type TestTarget struct {
	blockchain string
	chainID    int
}

type UserTestSuite struct {
	suite.Suite
	*IntegrationTestConfig
	target      TestTarget
	version     int
	liveWallets []*TestWallet
}

func setBlockchainContext(t TestTarget) {
	switch t.blockchain {
	case blockchainEth:
		switch t.chainID {
		case ethMainnet:
			viper.SetDefault("CONTRACT_ADDRESSES", contractAddressesEthMainnet)
			viper.SetDefault("CONTRACT_INTERACTION_URL", contractInteractionURLEthMainnet)
		case ethRinkeby:
			viper.SetDefault("CONTRACT_ADDRESSES", contractAddressesEthRinkeby)
			viper.SetDefault("CONTRACT_INTERACTION_URL", contractInteractionURLEthRinkeby)
		default:
			log.Fatal(errors.New("provided chainID is not supported"))
		}
	default:
		log.Fatal(errors.New("provided blockchain is not supported"))
	}
}

func (s *UserTestSuite) SetupTest() {
	setDefaults()
	setBlockchainContext(s.target)

	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("could not connect to docker: %s", err)
	}
	pg, pgClient := initPostgres(pool)
	rd := initRedis(pool)

	s.IntegrationTestConfig = &IntegrationTestConfig{
		TestConfig:    initializeTestServer(pgClient, s.Assertions, s.version),
		pool:          pool,
		pgResource:    pg,
		redisResource: rd,
		db:            pgClient,
	}
}

func (s *UserTestSuite) TearDownTest() {
	// Kill containers
	for _, r := range []*dockertest.Resource{s.pgResource, s.redisResource} {
		if err := s.pool.Purge(r); err != nil {
			log.Fatalf("could not purge resource: %s", err)
		}
	}

	s.db.Close()
	s.server.Close()
}

func (s *UserTestSuite) TestExistingUserCanLogin() {
	nonce := fetchNonce(s.Suite, s.serverURL, s.user1.address)
	loginUser(s.Suite, s.serverURL, nonce, s.user1.TestWallet)
}

func (s *UserTestSuite) TestEligibleWalletCanBecomeMember() {
	// create user
	nonce := fetchNonce(s.Suite, s.serverURL, s.liveWallets[0].address)
	createNewUser(s.Suite, s.serverURL, nonce, s.liveWallets[0])

	// login
	nonce = fetchNonce(s.Suite, s.serverURL, s.liveWallets[0].address)
	loginOutput := loginUser(s.Suite, s.serverURL, nonce, s.liveWallets[0])

	// get current user
	client := newClient()
	currentUserOutput := fetchCurrentUserIsValid(s.Suite, s.serverURL, client, loginOutput.JWTtoken)
	userOutput := fetchUser(s.Suite, s.serverURL, loginOutput.UserID)

	// logout
	resp := logoutUser(s.Suite, s.serverURL, client)
	jwtCookie := getCookieByName(auth.JWTCookieKey, resp.Cookies())
	s.NotEmpty(jwtCookie)

	// get current user
	afterLogout := fetchCurrentUserResponse(s.Suite, s.serverURL, client, jwtCookie.Value)

	s.Equal(loginOutput.UserID, currentUserOutput.UserID)
	s.Equal(loginOutput.UserID, userOutput.UserID)
	s.Equal(204, afterLogout.StatusCode)
}

func TestIntegrationTestUserTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	log.Infof("running integration tests against [%s:%d]", *blockchain, *chainID)
	suite.Run(t, &UserTestSuite{
		version:     1,
		liveWallets: []*TestWallet{loadWallet(*walletFile)},
		target:      TestTarget{*blockchain, *chainID},
	})
}
