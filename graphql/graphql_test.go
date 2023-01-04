package graphql_test

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	genql "github.com/Khan/genqlient/graphql"
	"net/http"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mitchellh/mapstructure"
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
		t.Run(test.title, withFixtures(test.run, test.fixtures...))
	}
}

func testGraphQL(t *testing.T) {
	tests := []testCase{
		{title: "should create a user", run: testCreateUser(newNonceFixture{})},
		{title: "should be able to login", run: testLogin(newUserFixture{})},
		{title: "should be able to logout", run: testLogout(newUserFixture{})},
		{title: "should get user by ID", run: testUserByID(newUserFixture{})},
		{title: "should get user by username", run: testUserByUsername(newUserFixture{})},
		{title: "should get user by address", run: testUserByAddress(newUserFixture{})},
		{title: "should get viewer", run: testViewer(newUserFixture{})},
		{title: "should add a wallet", run: testAddWallet(newUserFixture{})},
		{title: "should remove a wallet", run: testRemoveWallet(newUserFixture{})},
		{title: "should sync tokens", run: testSyncTokens(newUserFixture{})},
		{title: "should create a collection", run: testCreateCollection(newUserWithTokensFixture{})},
	}
	for _, test := range tests {
		t.Run(test.title, withFixtures(test.run, test.fixtures...))
	}
}

func testCreateUser(nonceF newNonceFixture) func(t *testing.T) {
	return func(t *testing.T) {
		nonceF.setup(t)
		c := defaultClientNew()
		username := "user" + persist.GenerateID().String()

		response, err := createUserMutation(context.Background(), c, AuthMechanism{
			Eoa: &EoaAuth{
				Nonce:     nonceF.nonce,
				Signature: nonceF.wallet.Sign(nonceF.nonce),
				ChainPubKey: ChainPubKeyInput{
					PubKey: nonceF.wallet.address,
					Chain:  "Ethereum",
				},
			},
		}, CreateUserInput{
			Username: username,
		})

		if err != nil {
			panic(err)
		}

		payload, _ := (*response.CreateUser).(*createUserMutationCreateUserCreateUserPayload)

		assert.Equal(t, username, *payload.Viewer.User.Username)
	}
}

func testUserByUsername(userF newUserFixture) func(t *testing.T) {
	return func(t *testing.T) {
		userF.setup(t)

		response, err := userByUsernameQuery(context.Background(), defaultClientNew(), userF.username)

		if err != nil {
			panic(err)
		}

		payload, _ := (*response.UserByUsername).(*userByUsernameQueryUserByUsernameGalleryUser)

		assert.Equal(t, userF.username, *payload.Username)
		assert.Equal(t, userF.id, payload.Dbid)
	}
}

func testUserByAddress(userF newUserFixture) func(t *testing.T) {
	return func(t *testing.T) {
		userF.setup(t)

		ctx := context.WithValue(context.Background(), "jwt", newJWT(t, userF.id))

		response, err := userByAddressQuery(ctx, defaultClientNew(), ChainAddressInput{
			Address: userF.wallet.address,
			Chain:   "Ethereum",
		})

		if err != nil {
			panic(err)
		}

		payload, _ := (*response.UserByAddress).(*userByAddressQueryUserByAddressGalleryUser)

		assert.Equal(t, userF.username, *payload.Username)
		assert.Equal(t, userF.id, payload.Dbid)
	}
}

func testUserByID(userF newUserFixture) func(t *testing.T) {
	return func(t *testing.T) {
		userF.setup(t)

		response, err := userByIdQuery(context.Background(), defaultClientNew(), userF.id)

		if err != nil {
			panic(err)
		}

		payload, _ := (*response.UserById).(*userByIdQueryUserByIdGalleryUser)

		assert.Equal(t, userF.username, *payload.Username)
		assert.Equal(t, userF.id, payload.Dbid)
	}
}

func testViewer(userF newUserFixture) func(t *testing.T) {
	return func(t *testing.T) {
		userF.setup(t)

		ctx := context.WithValue(context.Background(), "jwt", newJWT(t, userF.id))

		response, err := viewerQuery(ctx, defaultClientNew())

		if err != nil {
			panic(err)
		}

		payload, _ := (*response.Viewer).(*viewerQueryViewer)

		assert.Equal(t, userF.username, *payload.User.Username)
	}
}

func testAddWallet(userF newUserFixture) func(t *testing.T) {
	return func(t *testing.T) {
		userF.setup(t)

		walletToAdd := newWallet(t)
		c := defaultClientNew()

		nonce := newNonce(t, c, walletToAdd)

		ctx := context.WithValue(context.Background(), "jwt", newJWT(t, userF.id))

		response, err := addUserWalletMutation(ctx, defaultClientNew(), ChainAddressInput{
			Address: walletToAdd.address,
			Chain:   "Ethereum",
		}, AuthMechanism{
			Eoa: &EoaAuth{
				Nonce:     nonce,
				Signature: walletToAdd.Sign(nonce),
				ChainPubKey: ChainPubKeyInput{
					PubKey: walletToAdd.address,
					Chain:  "Ethereum",
				},
			},
		})

		if err != nil {
			panic(err)
		}

		payload, _ := (*response.AddUserWallet).(*addUserWalletMutationAddUserWalletAddUserWalletPayload)

		wallets := payload.Viewer.User.Wallets
		assert.Equal(t, walletToAdd.address, *wallets[len(wallets)-1].ChainAddress.Address)
		assert.Equal(t, Chain("Ethereum"), *wallets[len(wallets)-1].ChainAddress.Chain)
		assert.Len(t, wallets, 2)
	}
}

func testRemoveWallet(userF newUserFixture) func(t *testing.T) {
	return func(t *testing.T) {
		userF.setup(t)
		walletToRemove := newWallet(t)

		c := defaultClientNew()
		nonce := newNonce(t, c, walletToRemove)

		ctx := context.WithValue(context.Background(), "jwt", newJWT(t, userF.id))

		addResponse, err := addUserWalletMutation(ctx, defaultClientNew(), ChainAddressInput{
			Address: walletToRemove.address,
			Chain:   "Ethereum",
		}, AuthMechanism{
			Eoa: &EoaAuth{
				Nonce:     nonce,
				Signature: walletToRemove.Sign(nonce),
				ChainPubKey: ChainPubKeyInput{
					PubKey: walletToRemove.address,
					Chain:  "Ethereum",
				},
			},
		})

		if err != nil {
			panic(err)
		}

		wallets := (*addResponse.AddUserWallet).(*addUserWalletMutationAddUserWalletAddUserWalletPayload).Viewer.User.Wallets
		lastWallet := wallets[len(wallets)-1]
		assert.Len(t, wallets, 2)

		removeResponse, err := removeUserWalletsMutation(ctx, defaultClientNew(), []persist.DBID{lastWallet.Dbid})

		if err != nil {
			panic(err)
		}

		payload, _ := (*removeResponse.RemoveUserWallets).(*removeUserWalletsMutationRemoveUserWalletsRemoveUserWalletsPayload)

		assert.Len(t, payload.Viewer.User.Wallets, 1)
		assert.NotEqual(t, lastWallet.Dbid, payload.Viewer.User.Wallets[0].Dbid)
	}
}

func testLogin(userF newUserFixture) func(t *testing.T) {
	return func(t *testing.T) {
		userF.setup(t)
		nonce := newNonce(t, defaultClientNew(), userF.wallet)

		c := defaultClientNew()

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

		if err != nil {
			panic(err)
		}

		//TODO: TEST FOR COOKIE
		payload, _ := (*response.Login).(*loginMutationLoginLoginPayload)

		assert.Equal(t, userF.username, *payload.Viewer.User.Username)
		assert.Equal(t, userF.id, payload.Viewer.User.Dbid)
	}
}

func testLogout(userF newUserFixture) func(t *testing.T) {
	return func(t *testing.T) {
		userF.setup(t)

		c := defaultClientNew()
		ctx := context.WithValue(context.Background(), "jwt", newJWT(t, userF.id))

		response, err := logoutMutation(ctx, c)

		if err != nil {
			panic(err)
		}

		assert.Nil(t, response.Logout.Viewer)
	}
}

func testSyncTokens(userF newUserFixture) func(t *testing.T) {
	return func(t *testing.T) {
		userF.setup(t)
		clients := server.ClientInit(context.Background())
		p := multichain.Provider{
			Repos:       clients.Repos,
			TasksClient: clients.TaskClient,
			Queries:     clients.Queries,
			Chains:      map[persist.Chain][]interface{}{persist.ChainETH: {&stubProvider{}}},
		}

		h := server.CoreInit(clients, &p)

		c := genqlClient{
			handler:      newClient(h),
			lastResponse: nil,
		}

		ctx := context.WithValue(context.Background(), "jwt", newJWT(t, userF.id))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		if err != nil {
			panic(err)
		}

		payload := (*response.SyncTokens).(*syncTokensMutationSyncTokensSyncTokensPayload)

		assert.NotEmpty(t, payload.Viewer.User.Tokens)
	}
}

func testCreateCollection(userF newUserWithTokensFixture) func(*testing.T) {
	return func(t *testing.T) {
		userF.setup(t)
		c := defaultClientNew()
		ctx := context.WithValue(context.Background(), "jwt", newJWT(t, userF.id))

		response, err := createCollectionMutation(ctx, c, CreateCollectionInput{
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

		if err != nil {
			panic(err)
		}

		payload := (*response.CreateCollection).(*createCollectionMutationCreateCollectionCreateCollectionPayload)

		assert.NotEmpty(t, payload.Collection.Dbid)
		assert.Len(t, payload.Collection.Tokens, 1)
	}
}

// errMessage represents a handled GraphQL error
type errMessage struct {
	Typename string `json:"__typename"`
	Message  string `json:"message"`
}

// post makes a POST request using the provided client and decodes the response
// post will fail if an error is returned from the client or if decoding fails
func post(t *testing.T, c *client.Client, query string, into any, options ...client.Option) {
	t.Helper()
	r, err := c.RawPost(query, options...)
	require.NoError(t, err)
	require.Empty(t, string(r.Errors))

	d, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:      into,
		TagName:     "json",
		ErrorUnused: true,
		ZeroFields:  true,
		Squash:      true,
	})
	require.NoError(t, err)

	err = d.Decode(r.Data)
	require.NoError(t, err)
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

	if err != nil {
		panic(err)
	}

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

	if err != nil {
		panic(err)
	}

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

	c := genqlClient{
		handler:      newClient(handler),
		lastResponse: nil,
	}

	ctx := context.WithValue(context.Background(), "jwt", newJWT(t, userID))

	response, err := syncTokensMutation(ctx, c, []Chain{"Ethereum"})

	if err != nil {
		panic(err)
	}

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
func newClient(handler http.Handler) *client.Client {
	return client.New(handler, func(r *client.Request) {
		r.HTTP.URL.Path = "/glry/graphql/query"
	})
}

// defaultClient returns a GraphQL client attached to a backend GraphQL handler
func defaultClient() *client.Client {
	handler := defaultHandler()
	return newClient(handler)
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
		jsonSerializedIncoming, _ := json.Marshal(req.Variables)
		unmarshaled := map[string]any{}
		json.Unmarshal(jsonSerializedIncoming, &unmarshaled)

		bd.Variables = unmarshaled

		jwt := ctx.Value("jwt")
		if jwt != nil {
			bd.HTTP.AddCookie(&http.Cookie{Name: auth.JWTCookieKey, Value: jwt.(string)})
		}
	})

	if err != nil {
		return err
	}

	marshaledGqlgenResponse, err := json.Marshal(response.Data)
	if err != nil {
		return err
	}

	err = json.Unmarshal(marshaledGqlgenResponse, resp.Data)

	if err != nil {
		return err
	}

	c.lastResponse = resp

	return nil
}

func defaultClientNew() genqlClient {
	return genqlClient{handler: defaultClient()}
}
