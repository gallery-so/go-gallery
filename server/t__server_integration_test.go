package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"testing"

	migrate "github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/docker"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/ory/dockertest"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

const (
	// networks
	ethMainnet = iota + 1
	_
	ethRopsten
	ethRinkeby

	// eligible contracts
	contractAddressesEthMainnet = "0xe01569ca9b39E55Bc7C0dFa09F05fa15CB4C7698=[0,1,2,3,4,5,6,7,8]|0xe3d0fe9b7e0b951663267a3ed1e6577f6f79757e=[0]"
	contractAddressesEthRinkeby = "0x93eC9b03a9C14a530F582aef24a21d7FC88aaC46=[0,1,2,3,4,5,6,7,8]"

	// node providers
	contractInteractionURLEthMainnet = "https://eth-mainnet.alchemyapi.io/v2/_2u--i79yarLYdOT4Bgydqa0dBceVRLD"
	contractInteractionURLEthRinkeby = "https://eth-rinkeby.alchemyapi.io/v2/_2u--i79yarLYdOT4Bgydqa0dBceVRLD"

	// blockchains
	blockchainEth = "ethereum"

	// live wallets
	testWalletFileEthMainnet = "../_internal/test-wallet.json"
)

var (
	blockchain = flag.String("chain", blockchainEth, "blockchain to run against")
	chainID    = flag.Int("chainID", ethMainnet, "chainID to run against")
	walletFile = flag.String("walletFile", testWalletFileEthMainnet, "walletFile to load")
)

type IntegrationTestConfig struct {
	*TestConfig
	pgResource    *dockertest.Resource
	redisResource *dockertest.Resource
	db            *sql.DB
}

type TestTarget struct {
	blockchain string
	chainID    int
}

type IntegrationTest struct{}

// Tests related to user auth
type UserAuthSuite struct {
	suite.Suite
	*IntegrationTest
	*IntegrationTestConfig
	target      TestTarget
	version     int
	liveWallets []*TestWallet
}

// Test related to NFT collections (v1)
type NFTCollectionsSuite struct {
	suite.Suite
	*IntegrationTest
	*IntegrationTestConfig
	target      TestTarget
	version     int
	liveWallets []*TestWallet
}

// Test related to Token collections (v2)
type TokenCollectionsSuite struct {
	suite.Suite
	*IntegrationTest
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

func (i *IntegrationTest) setupTest(a *assert.Assertions, version int) *IntegrationTestConfig {
	pg := docker.InitPostgres("../docker-compose.yml")
	rd := docker.InitRedis("../docker-compose.yml")

	pgClient := postgres.NewClient()
	pgxClient := postgres.NewPgxClient()
	migrate.RunMigration("../db/migrations", pgClient)

	return &IntegrationTestConfig{
		TestConfig:    initializeTestServer(pgClient, pgxClient, a, version),
		pgResource:    pg,
		redisResource: rd,
		db:            pgClient,
	}
}

func (i *IntegrationTest) TearDownTest(tc *IntegrationTestConfig) {
	// Kill containers
	for _, r := range []*dockertest.Resource{tc.pgResource, tc.redisResource} {
		if err := r.Close(); err != nil {
			log.Fatalf("could not purge resource: %s", err)
		}
	}

	tc.db.Close()
	tc.server.Close()
}

func (s *UserAuthSuite) SetupTest() {
	setDefaults()
	setBlockchainContext(s.target)
	s.IntegrationTestConfig = s.setupTest(s.Assertions, s.version)
}

func (s *UserAuthSuite) TearDownTest() {
	s.IntegrationTest.TearDownTest(s.IntegrationTestConfig)
}

func (s *UserAuthSuite) TestExistingUserCanLogin() {
	nonce := fetchNonce(s.Suite, s.serverURL, s.user1.address)
	loginUser(s.Suite, s.serverURL, nonce, s.user1.TestWallet)
}

func (s *UserAuthSuite) TestEligibleWalletCanBecomeMember() {
	// create user
	nonce := fetchNonce(s.Suite, s.serverURL, s.liveWallets[0].address)
	createNewUser(s.Suite, s.serverURL, nonce, s.liveWallets[0])

	// login
	nonce = fetchNonce(s.Suite, s.serverURL, s.liveWallets[0].address)
	loginOutput, loginCookie := loginUser(s.Suite, s.serverURL, nonce, s.liveWallets[0])

	// get current user
	client := newClient()
	currentUserOutput := fetchCurrentUserIsValid(s.Suite, s.serverURL, client, loginCookie.Value)
	userOutput := fetchUser(s.Suite, s.serverURL, loginOutput.UserID)

	// logout
	resp := logoutUser(s.Suite, s.serverURL, client)
	logoutCookie := getCookieByName(auth.JWTCookieKey, resp.Cookies())
	s.NotEmpty(logoutCookie)

	// get current user
	afterLogout := fetchCurrentUserResponse(s.Suite, s.serverURL, client, logoutCookie.Value)

	s.Equal(loginOutput.UserID, currentUserOutput.UserID)
	s.Equal(loginOutput.UserID, userOutput.UserID)
	s.Equal(204, afterLogout.StatusCode)
}

func (s *NFTCollectionsSuite) SetupTest() {
	setDefaults()
	setBlockchainContext(s.target)
	s.IntegrationTestConfig = s.IntegrationTest.setupTest(s.Assertions, s.version)
}

func (s *NFTCollectionsSuite) TearDownTest() {
	s.IntegrationTest.TearDownTest(s.IntegrationTestConfig)
}

func (s *NFTCollectionsSuite) TestUserCanUpdateNFTCollection() {
	client := newClient()

	nonce := fetchNonce(s.Suite, s.serverURL, s.liveWallets[0].address)
	userOutput := createNewUser(s.Suite, s.serverURL, nonce, s.liveWallets[0])
	nonce = fetchNonce(s.Suite, s.serverURL, s.liveWallets[0].address)
	_, loginCookie := loginUser(s.Suite, s.serverURL, nonce, s.liveWallets[0])
	nftIDs := addNFTsToNFTsRepo(s.Suite, s.repos.NftRepository, s.liveWallets[0])

	// Create new collection
	data, err := json.Marshal(map[string]interface{}{"gallery_id": userOutput.GalleryID, "nfts": nftIDs})
	s.NoError(err)
	createOutput := createNewNFTCollection(s.Suite, s.serverURL, client, loginCookie.Value, data)
	collectionOutput := fetchNFTCollection(s.Suite, s.serverURL, createOutput.ID)
	s.Equal(createOutput.ID, collectionOutput.Collection.ID)

	// Change nothing
	data, err = json.Marshal(map[string]interface{}{"id": createOutput.ID, "nfts": nftIDs})
	s.NoError(err)
	resp := updateCollection(s.Suite, s.serverURL, client, loginCookie.Value, data)
	s.Equal(200, resp.StatusCode)

	changeOutput := fetchNFTCollection(s.Suite, s.serverURL, createOutput.ID)
	s.Equal(collectionOutput.Collection.ID, changeOutput.Collection.ID)
	s.Equal(collectionOutput.Collection.OwnerUserID, changeOutput.Collection.OwnerUserID)
	s.Equal(collectionOutput.Collection.NFTs, changeOutput.Collection.NFTs)
	s.Equal(collectionOutput.Collection.Layout, changeOutput.Collection.Layout)

	// Add a layout
	layout := persist.TokenLayout{Columns: 5}
	data, err = json.Marshal(map[string]interface{}{"id": createOutput.ID, "nfts": nftIDs, "layout": layout})
	s.NoError(err)
	resp = updateCollection(s.Suite, s.serverURL, client, loginCookie.Value, data)
	s.Equal(200, resp.StatusCode)

	changeOutput = fetchNFTCollection(s.Suite, s.serverURL, createOutput.ID)
	s.Equal(collectionOutput.Collection.ID, changeOutput.Collection.ID)
	s.Equal(collectionOutput.Collection.OwnerUserID, changeOutput.Collection.OwnerUserID)
	s.Equal(collectionOutput.Collection.NFTs, changeOutput.Collection.NFTs)
	s.Equal(changeOutput.Collection.Layout, layout)

	// Add whitespace to layout
	layout = persist.TokenLayout{Columns: 5, Whitespace: []int{0, 1, 2, 3}}
	data, err = json.Marshal(map[string]interface{}{"id": createOutput.ID, "nfts": nftIDs, "layout": layout})
	s.NoError(err)
	resp = updateCollection(s.Suite, s.serverURL, client, loginCookie.Value, data)
	s.Equal(200, resp.StatusCode)

	changeOutput = fetchNFTCollection(s.Suite, s.serverURL, createOutput.ID)
	s.Equal(collectionOutput.Collection.ID, changeOutput.Collection.ID)
	s.Equal(collectionOutput.Collection.OwnerUserID, changeOutput.Collection.OwnerUserID)
	s.Equal(collectionOutput.Collection.NFTs, changeOutput.Collection.NFTs)
	s.Equal(changeOutput.Collection.Layout, layout)

	// Delete collection
	data, err = json.Marshal(map[string]interface{}{"id": createOutput.ID})
	s.NoError(err)
	deleteOutput := removeCollection(s.Suite, s.serverURL, client, loginCookie.Value, data)
	s.Equal(200, resp.StatusCode)
	s.True(deleteOutput.Success)

	resp = getNFTCollection(s.Suite, s.serverURL, createOutput.ID)
	s.Equal(404, resp.StatusCode)
}

func (s *TokenCollectionsSuite) SetupTest() {
	setDefaults()
	setBlockchainContext(s.target)
	s.IntegrationTestConfig = s.setupTest(s.Assertions, s.version)
}

func (s *TokenCollectionsSuite) TearDownTest() {
	s.IntegrationTest.TearDownTest(s.IntegrationTestConfig)
}

func (s *TokenCollectionsSuite) TestUserCanUpdateTokenCollection() {
	client := newClient()

	nonce := fetchNonce(s.Suite, s.serverURL, s.liveWallets[0].address)
	userOutput := createNewUser(s.Suite, s.serverURL, nonce, s.liveWallets[0])
	nonce = fetchNonce(s.Suite, s.serverURL, s.liveWallets[0].address)
	_, loginCookie := loginUser(s.Suite, s.serverURL, nonce, s.liveWallets[0])
	tokenIDs := addTokensToTokensRepo(s.Suite, s.repos.TokenRepository, s.liveWallets[0])

	// Create new collection
	data, err := json.Marshal(map[string]interface{}{"gallery_id": userOutput.GalleryID, "nfts": tokenIDs})
	s.NoError(err)
	createOutput := createNewTokenCollection(s.Suite, s.serverURL, client, loginCookie.Value, data)
	collectionOutput := fetchTokenCollection(s.Suite, s.serverURL, createOutput.ID)
	s.Equal(createOutput.ID, collectionOutput.Collection.ID)

	// Change nothing
	data, err = json.Marshal(map[string]interface{}{"id": createOutput.ID, "nfts": tokenIDs})
	s.NoError(err)
	resp := updateCollection(s.Suite, s.serverURL, client, loginCookie.Value, data)
	s.Equal(200, resp.StatusCode)

	changeOutput := fetchTokenCollection(s.Suite, s.serverURL, createOutput.ID)
	s.Equal(collectionOutput.Collection.ID, changeOutput.Collection.ID)
	s.Equal(collectionOutput.Collection.OwnerUserID, changeOutput.Collection.OwnerUserID)
	s.Equal(collectionOutput.Collection.NFTs, changeOutput.Collection.NFTs)
	s.Equal(collectionOutput.Collection.Layout, changeOutput.Collection.Layout)

	// Add a layout
	layout := persist.TokenLayout{Columns: 5}
	data, err = json.Marshal(map[string]interface{}{"id": createOutput.ID, "nfts": tokenIDs, "layout": layout})
	s.NoError(err)
	resp = updateCollection(s.Suite, s.serverURL, client, loginCookie.Value, data)
	s.Equal(200, resp.StatusCode)

	changeOutput = fetchTokenCollection(s.Suite, s.serverURL, createOutput.ID)
	s.Equal(collectionOutput.Collection.ID, changeOutput.Collection.ID)
	s.Equal(collectionOutput.Collection.OwnerUserID, changeOutput.Collection.OwnerUserID)
	s.Equal(collectionOutput.Collection.NFTs, changeOutput.Collection.NFTs)
	s.Equal(changeOutput.Collection.Layout, layout)

	// Add whitespace to layout
	layout = persist.TokenLayout{Columns: 5, Whitespace: []int{0, 1, 2, 3}}
	data, err = json.Marshal(map[string]interface{}{"id": createOutput.ID, "nfts": tokenIDs, "layout": layout})
	s.NoError(err)
	resp = updateCollection(s.Suite, s.serverURL, client, loginCookie.Value, data)
	s.Equal(200, resp.StatusCode)

	changeOutput = fetchTokenCollection(s.Suite, s.serverURL, createOutput.ID)
	s.Equal(collectionOutput.Collection.ID, changeOutput.Collection.ID)
	s.Equal(collectionOutput.Collection.OwnerUserID, changeOutput.Collection.OwnerUserID)
	s.Equal(collectionOutput.Collection.NFTs, changeOutput.Collection.NFTs)
	s.Equal(changeOutput.Collection.Layout, layout)

	// Delete collection
	data, err = json.Marshal(map[string]interface{}{"id": createOutput.ID})
	s.NoError(err)
	deleteOutput := removeCollection(s.Suite, s.serverURL, client, loginCookie.Value, data)
	s.Equal(200, resp.StatusCode)
	s.True(deleteOutput.Success)

	resp = getNFTCollection(s.Suite, s.serverURL, createOutput.ID)
	s.Equal(404, resp.StatusCode)
}

func TestIntegrationTestTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	log.Infof("running integration tests against [%s:%d]", *blockchain, *chainID)
	wallet := loadWallet(*walletFile)
	suite.Run(t, &UserAuthSuite{
		version:     1,
		liveWallets: []*TestWallet{wallet},
		target:      TestTarget{*blockchain, *chainID},
	})
	suite.Run(t, &NFTCollectionsSuite{
		version:     1,
		liveWallets: []*TestWallet{wallet},
		target:      TestTarget{*blockchain, *chainID},
	})
	suite.Run(t, &TokenCollectionsSuite{
		version:     2,
		liveWallets: []*TestWallet{wallet},
		target:      TestTarget{*blockchain, *chainID},
	})
}
