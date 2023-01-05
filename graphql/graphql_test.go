package graphql_test

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	genql "github.com/Khan/genqlient/graphql"

	"github.com/99designs/gqlgen/client"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testCase struct {
	title    string
	run      func(t *testing.T)
	fixtures []fixture
}

func TestMain(t *testing.T) {
	tests := []testCase{
		{
			title:    "test GraphQL",
			run:      testGraphQL,
			fixtures: []fixture{useDefaultEnv, usePostgres, useRedis, useTokenQueue},
		},
	}
	for _, test := range tests {
		t.Run(test.title, testWithFixtures(test.run, test.fixtures...))
	}
}

func testGraphQL(t *testing.T) {
	tests := []testCase{
		{title: "should create a user", run: testCreateUser},
		{title: "should be able to login", run: testLogin},
		{title: "should be able to logout", run: testLogout},
		{title: "should get user by ID", run: testUserByID},
		{title: "should get user by username", run: testUserByUsername},
		{title: "should get user by address", run: testUserByAddress},
		{title: "should get viewer", run: testViewer},
		{title: "should add a wallet", run: testAddWallet},
		{title: "should remove a wallet", run: testRemoveWallet},
		{title: "should sync tokens", run: testSyncTokens},
		{title: "should create a collection", run: testCreateCollection},
	}
	for _, test := range tests {
		t.Run(test.title, testWithFixtures(test.run, test.fixtures...))
	}
}

func testCreateUser(t *testing.T) {
	nonceF := newNonceFixture(t)
	c := defaultClient(t)
	username := "user" + persist.GenerateID().String()

	response, err := createUserMutation(context.Background(), c, eoaAuthMechanismInput(nonceF.wallet, nonceF.nonce),
		CreateUserInput{
			Username: username,
		},
	)

	require.NoError(t, err)
	payload, _ := (*response.CreateUser).(*createUserMutationCreateUserCreateUserPayload)
	assert.Equal(t, username, *payload.Viewer.User.Username)
}

func testUserByUsername(t *testing.T) {
	userF := newUserFixture(t)

	response, err := userByUsernameQuery(context.Background(), defaultClient(t), userF.username)

	require.NoError(t, err)
	payload, _ := (*response.UserByUsername).(*userByUsernameQueryUserByUsernameGalleryUser)
	assert.Equal(t, userF.username, *payload.Username)
	assert.Equal(t, userF.id, payload.Dbid)
}

func testUserByAddress(t *testing.T) {
	userF := newUserFixture(t)
	c := authedClient(t, userF.id)

	response, err := userByAddressQuery(context.Background(), c, ChainAddressInput{
		Address: userF.wallet.address,
		Chain:   "Ethereum",
	})
	require.NoError(t, err)

	payload, _ := (*response.UserByAddress).(*userByAddressQueryUserByAddressGalleryUser)
	assert.Equal(t, userF.username, *payload.Username)
	assert.Equal(t, userF.id, payload.Dbid)
}

func testUserByID(t *testing.T) {
	userF := newUserFixture(t)

	response, err := userByIdQuery(context.Background(), defaultClient(t), userF.id)

	require.NoError(t, err)
	payload, _ := (*response.UserById).(*userByIdQueryUserByIdGalleryUser)
	assert.Equal(t, userF.username, *payload.Username)
	assert.Equal(t, userF.id, payload.Dbid)
}

func testViewer(t *testing.T) {
	userF := newUserFixture(t)
	c := authedClient(t, userF.id)

	response, err := viewerQuery(context.Background(), c)

	require.NoError(t, err)
	payload, _ := (*response.Viewer).(*viewerQueryViewer)
	assert.Equal(t, userF.username, *payload.User.Username)
}

func testAddWallet(t *testing.T) {
	userF := newUserFixture(t)
	walletToAdd := newWallet(t)
	c := authedClient(t, userF.id)
	nonce := newNonce(t, c, walletToAdd)

	response, err := addUserWalletMutation(context.Background(), c, ChainAddressInput{
		Address: walletToAdd.address,
		Chain:   "Ethereum",
	}, eoaAuthMechanismInput(walletToAdd, nonce))

	require.NoError(t, err)
	payload, _ := (*response.AddUserWallet).(*addUserWalletMutationAddUserWalletAddUserWalletPayload)
	wallets := payload.Viewer.User.Wallets
	assert.Equal(t, walletToAdd.address, *wallets[len(wallets)-1].ChainAddress.Address)
	assert.Equal(t, Chain("Ethereum"), *wallets[len(wallets)-1].ChainAddress.Chain)
	assert.Len(t, wallets, 2)
}

func testRemoveWallet(t *testing.T) {
	userF := newUserFixture(t)
	walletToRemove := newWallet(t)
	c := authedClient(t, userF.id)
	nonce := newNonce(t, c, walletToRemove)
	addResponse, err := addUserWalletMutation(context.Background(), c, ChainAddressInput{
		Address: walletToRemove.address,
		Chain:   "Ethereum",
	}, eoaAuthMechanismInput(walletToRemove, nonce))
	require.NoError(t, err)
	wallets := (*addResponse.AddUserWallet).(*addUserWalletMutationAddUserWalletAddUserWalletPayload).Viewer.User.Wallets
	lastWallet := wallets[len(wallets)-1]
	assert.Len(t, wallets, 2)

	removeResponse, err := removeUserWalletsMutation(context.Background(), c, []persist.DBID{lastWallet.Dbid})

	require.NoError(t, err)
	payload, _ := (*removeResponse.RemoveUserWallets).(*removeUserWalletsMutationRemoveUserWalletsRemoveUserWalletsPayload)
	assert.Len(t, payload.Viewer.User.Wallets, 1)
	assert.NotEqual(t, lastWallet.Dbid, payload.Viewer.User.Wallets[0].Dbid)
}

func testLogin(t *testing.T) {
	userF := newUserFixture(t)
	c := defaultClient(t)
	nonce := newNonce(t, c, userF.wallet)

	response, err := loginMutation(context.Background(), c, AuthMechanism{
		Eoa: &EoaAuth{
			Nonce:     nonce,
			Signature: userF.wallet.Sign(nonce),
			ChainPubKey: ChainPubKeyInput{
				PubKey: userF.wallet.address,
				Chain:  "Ethereum",
			},
		},
	})

	//TODO: TEST FOR COOKIE
	require.NoError(t, err)
	payload, _ := (*response.Login).(*loginMutationLoginLoginPayload)
	assert.Equal(t, userF.username, *payload.Viewer.User.Username)
	assert.Equal(t, userF.id, payload.Viewer.User.Dbid)
}

func testLogout(t *testing.T) {
	userF := newUserFixture(t)
	c := authedClient(t, userF.id)

	response, err := logoutMutation(context.Background(), c)

	//TODO: TEST FOR NO COOKIE
	require.NoError(t, err)
	assert.Nil(t, response.Logout.Viewer)
}

func testSyncTokens(t *testing.T) {
	userF := newUserFixture(t)
	clients := server.ClientInit(context.Background())
	p := multichain.Provider{
		Repos:       clients.Repos,
		TasksClient: clients.TaskClient,
		Queries:     clients.Queries,
		Chains:      map[persist.Chain][]interface{}{persist.ChainETH: {&stubProvider{}}},
	}
	h := server.CoreInit(clients, &p)
	c := customClient(t, h, withJWTOpt(t, userF.id))

	response, err := syncTokensMutation(context.Background(), c, []Chain{ChainEthereum})

	require.NoError(t, err)
	payload := (*response.SyncTokens).(*syncTokensMutationSyncTokensSyncTokensPayload)
	assert.NotEmpty(t, payload.Viewer.User.Tokens)
}

func testCreateCollection(t *testing.T) {
	userF := newUserWithTokensFixture(t)
	c := authedClient(t, userF.id)

	response, err := createCollectionMutation(context.Background(), c, CreateCollectionInput{
		GalleryId:      userF.galleryID,
		Name:           "newCollection",
		CollectorsNote: "this is a note",
		Tokens:         userF.tokenIDs[:1],
		Layout: CollectionLayoutInput{
			Sections: []int{0},
			SectionLayout: []CollectionSectionLayoutInput{
				{
					Columns:    0,
					Whitespace: []int{},
				},
			},
		},
		TokenSettings: []CollectionTokenSettingsInput{
			{
				TokenId:    userF.tokenIDs[0],
				RenderLive: false,
			},
		},
		Caption: nil,
	})

	require.NoError(t, err)
	payload := (*response.CreateCollection).(*createCollectionMutationCreateCollectionCreateCollectionPayload)
	assert.NotEmpty(t, payload.Collection.Dbid)
	assert.Len(t, payload.Collection.Tokens, 1)
}

// eoaAuthMechanismInput signs a nonce with an ethereum wallet
func eoaAuthMechanismInput(w wallet, nonce string) AuthMechanism {
	return AuthMechanism{
		Eoa: &EoaAuth{
			Nonce:     nonce,
			Signature: w.Sign(nonce),
			ChainPubKey: ChainPubKeyInput{
				PubKey: w.address,
				Chain:  "Ethereum",
			},
		},
	}
}

type wallet struct {
	pKey    *ecdsa.PrivateKey
	pubKey  *ecdsa.PublicKey
	address string
}

func (w *wallet) Sign(msg string) string {
	sig, err := crypto.Sign(crypto.Keccak256([]byte(msg)), w.pKey)
	if err != nil {
		panic(err)
	}
	return "0x" + hex.EncodeToString(sig)
}

// newWallet generates a new wallet for testing purposes
func newWallet(t *testing.T) wallet {
	t.Helper()
	pk, err := crypto.GenerateKey()
	require.NoError(t, err)

	pubKey := pk.Public().(*ecdsa.PublicKey)
	address := strings.ToLower(crypto.PubkeyToAddress(*pubKey).Hex())

	return wallet{
		pKey:    pk,
		pubKey:  pubKey,
		address: address,
	}
}

// newNonce makes a GraphQL request to generate a nonce
func newNonce(t *testing.T, c genqlClient, w wallet) string {
	t.Helper()
	response, err := getAuthNonceMutation(context.Background(), c, ChainAddressInput{
		Address: w.address,
		Chain:   "Ethereum",
	})
	require.NoError(t, err)
	payload := (*response.GetAuthNonce).(*getAuthNonceMutationGetAuthNonce)
	return *payload.Nonce
}

// newUser makes a GraphQL request to generate a new user
func newUser(t *testing.T, c genqlClient, w wallet) (userID persist.DBID, username string, galleryID persist.DBID) {
	t.Helper()
	nonce := newNonce(t, c, w)
	username = "user" + persist.GenerateID().String()

	response, err := createUserMutation(context.Background(), c, AuthMechanism{
		Eoa: &EoaAuth{
			Nonce:     nonce,
			Signature: w.Sign(nonce),
			ChainPubKey: ChainPubKeyInput{
				PubKey: w.address,
				Chain:  "Ethereum",
			},
		},
	}, CreateUserInput{Username: username})

	require.NoError(t, err)
	payload := (*response.CreateUser).(*createUserMutationCreateUserCreateUserPayload)
	return payload.Viewer.User.Dbid, username, payload.Viewer.User.Galleries[0].Dbid
}

// newJWT generates a JWT
func newJWT(t *testing.T, userID persist.DBID) string {
	jwt, err := auth.JWTGeneratePipeline(context.Background(), userID)
	require.NoError(t, err)
	return jwt
}

// syncTokens makes a GraphQL request to sync a user's wallet
func syncTokens(t *testing.T, handler http.Handler, userID persist.DBID) []persist.DBID {
	t.Helper()
	c := customClient(t, handler, withJWTOpt(t, userID))

	response, err := syncTokensMutation(context.Background(), c, []Chain{"Ethereum"})

	require.NoError(t, err)
	payload := (*response.SyncTokens).(*syncTokensMutationSyncTokensSyncTokensPayload)
	tokens := make([]persist.DBID, len(payload.Viewer.User.Tokens))
	for i, token := range payload.Viewer.User.Tokens {
		tokens[i] = token.Dbid
	}
	return tokens
}

// defaultHandler returns a backend GraphQL http.Handler
func defaultHandler() http.Handler {
	c := server.ClientInit(context.Background())
	p := server.NewMultichainProvider(c)
	handler := server.CoreInit(c, p)
	return handler
}

// newClient returns a gqlgen test client
func newClient(handler http.Handler, opts ...client.Option) *client.Client {
	defaultOpts := []client.Option{gqlPathOpt}
	opts = append(opts, defaultOpts...)
	return client.New(handler, opts...)
}

// defaultClient returns a GraphQL client attached to a backend GraphQL handler
func defaultClient(t *testing.T) genqlClient {
	handler := defaultHandler()
	return customClient(t, handler)
}

// authedClient returns a GraphQL client with an authenticated JWT
func authedClient(t *testing.T, userID persist.DBID) genqlClient {
	handler := defaultHandler()
	return customClient(t, handler, withJWTOpt(t, userID))
}

// customClient configures the client with the provided HTTP handler and client options
func customClient(t *testing.T, handler http.Handler, opts ...client.Option) genqlClient {
	client := newClient(handler, opts...)
	return genqlClient{handler: client}
}

// gqlPathOpt sets all client's request paths to the GraphQL endpoint
func gqlPathOpt(r *client.Request) {
	r.HTTP.URL.Path = "/glry/graphql/query"
}

// withJWTOpt ddds a JWT cookie to the request headers
func withJWTOpt(t *testing.T, userID persist.DBID) func(*client.Request) {
	jwt, err := auth.JWTGeneratePipeline(context.Background(), userID)
	require.NoError(t, err)
	return func(r *client.Request) {
		r.HTTP.AddCookie(&http.Cookie{Name: auth.JWTCookieKey, Value: jwt})
	}
}

type genqlClient struct {
	handler      *client.Client
	lastResponse *genql.Response
}

func (c genqlClient) MakeRequest(
	ctx context.Context,
	req *genql.Request,
	resp *genql.Response,
) error {
	response, err := c.handler.RawPost(req.Query, func(bd *client.Request) {
		marshalledVars, _ := json.Marshal(req.Variables)
		unmarshalledVars := map[string]any{}
		json.Unmarshal(marshalledVars, &unmarshalledVars)
		bd.Variables = unmarshalledVars
	})
	if err != nil {
		return err
	}

	marshalledResponse, err := json.Marshal(response.Data)
	if err != nil {
		return err
	}

	err = json.Unmarshal(marshalledResponse, resp.Data)
	if err != nil {
		return err
	}

	c.lastResponse = resp

	return nil
}
