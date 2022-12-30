package graphql_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/mikeydub/go-gallery/graphql/generated"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

var ops = loadOperations(util.MustFindFile("./testdata/operations.gql"))

type testCase struct {
	title    string
	run      func(t *testing.T)
	fixtures []fixture
}

func TestGraphQL(t *testing.T) {
	tests := []testCase{
		{
			title:    "test user API",
			run:      testGraphQL_User,
			fixtures: []fixture{useDefaultEnv, usePostgres, useRedis},
		},
		{
			title:    "test tokens API",
			run:      testGraphQL_Tokens,
			fixtures: []fixture{useDefaultEnv, usePostgres, useRedis, useCloudTasks},
		},
		{
			title:    "test collections API",
			run:      testGraphQL_Collections,
			fixtures: []fixture{useDefaultEnv, usePostgres, useRedis, useCloudTasks},
		},
	}
	for _, test := range tests {
		t.Run(test.title, withFixtures(test.run, test.fixtures...))
	}
}

func testGraphQL_User(t *testing.T) {
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
	}
	for _, test := range tests {
		t.Run(test.title, withFixtures(test.run, test.fixtures...))
	}
}

func testGraphQL_Tokens(t *testing.T) {
	tests := []testCase{
		{title: "should sync tokens", run: testSyncTokens(newUserFixture{})},
	}
	for _, test := range tests {
		t.Run(test.title, withFixtures(test.run, test.fixtures...))
	}
}

func testGraphQL_Collections(t *testing.T) {
	tests := []testCase{
		{title: "should create a collection", run: testCreateCollection(newUserWithTokensFixture{})},
	}
	for _, test := range tests {
		t.Run(test.title, withFixtures(test.run, test.fixtures...))
	}
}

func testCreateUser(nonceF newNonceFixture) func(t *testing.T) {
	return func(t *testing.T) {
		nonceF.setup(t)
		c := defaultClient()
		username := "user" + persist.GenerateID().String()
		var response = struct {
			CreateUser struct {
				Viewer model.Viewer
				errMessage
			}
		}{}

		post(t, c, ops.Op("createUserMutation"), &response,
			client.Var("username", username),
			client.Var("authMechanism", map[string]any{
				"eoa": map[string]any{
					"nonce":       nonceF.nonce,
					"signature":   nonceF.wallet.Sign(nonceF.nonce),
					"chainPubKey": map[string]string{"pubKey": nonceF.wallet.address, "chain": "Ethereum"},
				},
			}),
		)

		require.Empty(t, response.CreateUser.Message)
		assert.Equal(t, username, *response.CreateUser.Viewer.User.Username)
	}
}

func testUserByUsername(userF newUserFixture) func(t *testing.T) {
	return func(t *testing.T) {
		userF.setup(t)
		c := defaultClient()
		var response = struct {
			UserByUsername struct {
				model.GalleryUser
				errMessage
			}
		}{}

		post(t, c, ops.Op("userByUsernameQuery"), &response, client.Var("user", userF.username))

		require.Empty(t, response.UserByUsername.Message)
		assert.Equal(t, userF.username, *response.UserByUsername.Username)
		assert.Equal(t, userF.id, response.UserByUsername.Dbid)
	}
}

func testUserByAddress(userF newUserFixture) func(t *testing.T) {
	return func(t *testing.T) {
		userF.setup(t)
		c := defaultClient()
		var response = struct {
			UserByAddress struct {
				model.GalleryUser
				errMessage
			}
		}{}

		post(t, c, ops.Op("userByAddressQuery"), &response,
			client.Var("input", map[string]string{
				"address": userF.wallet.address,
				"chain":   "Ethereum",
			}),
		)

		require.Empty(t, response.UserByAddress.Message)
		assert.Equal(t, userF.username, *response.UserByAddress.Username)
		assert.Equal(t, userF.id, response.UserByAddress.Dbid)
	}
}

func testUserByID(userF newUserFixture) func(t *testing.T) {
	return func(t *testing.T) {
		userF.setup(t)
		c := defaultClient()
		var response = struct {
			UserByID struct {
				model.GalleryUser
				errMessage
			}
		}{}

		post(t, c, ops.Op("userByIdQuery"), &response, client.Var("id", userF.id))

		require.Empty(t, response.UserByID.Message)
		assert.Equal(t, userF.username, *response.UserByID.Username)
		assert.Equal(t, userF.id, response.UserByID.Dbid)
	}
}

func testViewer(userF newUserFixture) func(t *testing.T) {
	return func(t *testing.T) {
		userF.setup(t)
		c := defaultClient()
		var response = struct {
			Viewer struct {
				model.Viewer
				errMessage
			}
		}{}

		post(t, c, ops.Op("viewerQuery"), &response, withJWT(newJWT(t, userF.id)))

		require.Empty(t, response.Viewer.Message)
		assert.Equal(t, userF.username, *response.Viewer.User.Username)
	}
}

func testAddWallet(userF newUserFixture) func(t *testing.T) {
	return func(t *testing.T) {
		userF.setup(t)
		c := defaultClient()
		walletToAdd := newWallet(t)
		nonce := newNonce(t, c, walletToAdd)
		var response = struct {
			AddUserWallet struct {
				errMessage
				Viewer struct {
					User struct {
						Wallets []struct {
							Dbid         string
							ChainAddress struct {
								Address string
								Chain   string
							}
						}
					}
				}
			}
		}{}

		post(t, c, ops.Op("addUserWalletMutation"), &response,
			withJWT(newJWT(t, userF.id)),
			client.Var("chainAddress", map[string]string{
				"address": walletToAdd.address,
				"chain":   "Ethereum",
			}),
			client.Var(
				"authMechanism", map[string]any{
					"eoa": map[string]any{
						"nonce":       nonce,
						"signature":   walletToAdd.Sign(nonce),
						"chainPubKey": map[string]string{"pubKey": walletToAdd.address, "chain": "Ethereum"},
					},
				},
			),
		)

		require.Empty(t, response.AddUserWallet.Message)
		wallets := response.AddUserWallet.Viewer.User.Wallets
		assert.Equal(t, walletToAdd.address, wallets[len(wallets)-1].ChainAddress.Address)
		assert.Equal(t, "Ethereum", wallets[len(wallets)-1].ChainAddress.Chain)
		assert.Len(t, wallets, 2)
	}
}

func testRemoveWallet(userF newUserFixture) func(t *testing.T) {
	return func(t *testing.T) {
		userF.setup(t)
		c := defaultClient()
		walletToRemove := newWallet(t)
		nonce := newNonce(t, c, walletToRemove)
		jwt := newJWT(t, userF.id)
		// First add a wallet
		var addResponse = struct {
			AddUserWallet struct {
				errMessage
				Viewer struct {
					User struct {
						Wallets []struct {
							Dbid         string
							ChainAddress struct {
								Address string
								Chain   string
							}
						}
					}
				}
			}
		}{}
		post(t, c, ops.Op("addUserWalletMutation"), &addResponse,
			withJWT(jwt),
			client.Var("chainAddress", map[string]string{
				"address": walletToRemove.address,
				"chain":   "Ethereum",
			}),
			client.Var(
				"authMechanism", map[string]any{
					"eoa": map[string]any{
						"nonce":       nonce,
						"signature":   walletToRemove.Sign(nonce),
						"chainPubKey": map[string]string{"pubKey": walletToRemove.address, "chain": "Ethereum"},
					},
				},
			),
		)
		require.Empty(t, addResponse.AddUserWallet.Message)
		wallets := addResponse.AddUserWallet.Viewer.User.Wallets
		lastWallet := wallets[len(wallets)-1]
		assert.Len(t, wallets, 2)

		// Then remove the wallet
		var removeResponse = struct {
			RemoveUserWallets struct {
				errMessage
				Viewer struct {
					User struct {
						Wallets []struct {
							Dbid         string
							ChainAddress struct {
								Address string
								Chain   string
							}
						}
					}
				}
			}
		}{}
		post(t, c, ops.Op("removeUserWalletsMutation"), &removeResponse,
			withJWT(jwt),
			client.Var("walletIds", []string{lastWallet.Dbid}),
		)

		require.Empty(t, removeResponse.RemoveUserWallets.Message)
		assert.Len(t, removeResponse.RemoveUserWallets.Viewer.User.Wallets, 1)
		assert.NotEqual(t, lastWallet.Dbid, removeResponse.RemoveUserWallets.Viewer.User.Wallets[0].Dbid)
	}
}

func testLogin(userF newUserFixture) func(t *testing.T) {
	return func(t *testing.T) {
		userF.setup(t)
		nonce := newNonce(t, defaultClient(), userF.wallet)
		// Manually create the request so that we can write to a recorder
		body, _ := json.Marshal(map[string]any{
			"query": ops.Op("loginMutation"),
			"variables": map[string]any{
				"authMechanism": map[string]any{
					"eoa": map[string]any{
						"nonce":       nonce,
						"signature":   userF.wallet.Sign(nonce),
						"chainPubKey": map[string]string{"pubKey": userF.wallet.address, "chain": "Ethereum"},
					},
				},
			},
		})
		r := httptest.NewRequest(http.MethodPost, "/glry/graphql/query", io.NopCloser(bytes.NewBuffer(body)))
		r.Header.Set("Content-Type", "application/json")

		// Handle request
		w := httptest.NewRecorder()
		handler := defaultHandler()
		handler.ServeHTTP(w, r)
		res := w.Result()
		defer res.Body.Close()

		// Check results
		var response = struct {
			Data struct {
				Login struct {
					Viewer model.Viewer
					errMessage
				}
			}
		}{}
		buf := new(bytes.Buffer)
		buf.ReadFrom(res.Body)
		err := json.Unmarshal(buf.Bytes(), &response)
		require.NoError(t, err)
		require.Empty(t, response.Data.Login.Message)
		assert.NotEmpty(t, readCookie(t, res, auth.JWTCookieKey))
		assert.Equal(t, userF.username, *response.Data.Login.Viewer.User.Username)
		assert.Equal(t, userF.id, response.Data.Login.Viewer.User.Dbid)
	}
}

func testLogout(userF newUserFixture) func(t *testing.T) {
	return func(t *testing.T) {
		userF.setup(t)
		// Manually create the request so that we can write to a recorder
		body, _ := json.Marshal(map[string]string{"query": ops.Op("logoutMutation")})
		r := httptest.NewRequest(http.MethodPost, "/glry/graphql/query", io.NopCloser(bytes.NewBuffer(body)))
		r.Header.Set("Content-Type", "application/json")
		addJWT(r, newJWT(t, userF.id))

		// Handle request
		w := httptest.NewRecorder()
		handler := defaultHandler()
		handler.ServeHTTP(w, r)
		res := w.Result()
		defer res.Body.Close()

		// Check results
		buf := new(bytes.Buffer)
		buf.ReadFrom(res.Body)
		var response = struct {
			Logout struct {
				Viewer model.Viewer
			}
		}{}
		err := json.Unmarshal(buf.Bytes(), &response)
		require.NoError(t, err)
		assert.Empty(t, readCookie(t, res, auth.JWTCookieKey))
		assert.Empty(t, response.Logout.Viewer)
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
		c := newClient(h)
		var response = struct {
			SyncTokens struct {
				errMessage
				Viewer struct {
					User struct {
						Tokens []struct {
							Chain   string
							DBID    persist.DBID
							TokenID string
						}
					}
				}
			}
		}{}

		post(t, c, ops.Op("syncTokensMutation"), &response,
			withJWT(newJWT(t, userF.id)),
			client.Var("walletIds", []map[string]string{
				{"address": userF.wallet.address, "chain": "Ethereum"},
			}),
		)

		require.Empty(t, response.SyncTokens.Message)
		assert.NotEmpty(t, response.SyncTokens.Viewer.User.Tokens)
	}
}

func testCreateCollection(userF newUserWithTokensFixture) func(*testing.T) {
	return func(t *testing.T) {
		userF.setup(t)
		c := defaultClient()

		var response = struct {
			CreateCollection struct {
				model.CreateCollectionPayload
				errMessage
			}
		}{}

		post(t, c, ops.Op("createCollectionMutation"), &response,
			withJWT(newJWT(t, userF.id)),
			client.Var("input", map[string]any{
				"galleryId":      userF.galleryID,
				"name":           "newCollection",
				"tokens":         userF.tokenIDs[:1],
				"collectorsNote": "this is a note",
				"layout": map[string]any{
					"sections": []int{0},
					"sectionLayout": map[string]any{
						"columns":    0,
						"whitespace": []int{},
					},
				},
				"tokenSettings": []map[string]any{
					{
						"tokenId":    userF.tokenIDs[0],
						"renderLive": false,
					},
				},
			}),
		)

		require.Empty(t, response.CreateCollection.Message)
		assert.NotEmpty(t, response.CreateCollection.Collection.Dbid)
		assert.Len(t, response.CreateCollection.Collection.Tokens, 1)
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
func newNonce(t *testing.T, c *client.Client, w wallet) string {
	t.Helper()
	response := struct {
		GetAuthNonce struct {
			model.AuthNonce
			errMessage
		}
	}{}

	post(t, c, ops.Op("getAuthNonceMutation"), &response,
		client.Var("input", map[string]string{
			"address": w.address,
			"chain":   "Ethereum",
		}),
	)
	require.Empty(t, response.GetAuthNonce.Message)

	return *response.GetAuthNonce.Nonce
}

// newUser makes a GraphQL request to generate a new user
func newUser(t *testing.T, c *client.Client, w wallet) (userID persist.DBID, username string, galleryID persist.DBID) {
	t.Helper()
	nonce := newNonce(t, c, w)
	username = "user" + persist.GenerateID().String()
	var response = struct {
		CreateUser struct {
			Viewer model.Viewer
			errMessage
		}
	}{}

	post(t, c, ops.Op("createUserMutation"), &response,
		client.Var("username", username),
		client.Var("authMechanism", map[string]any{
			"eoa": map[string]any{
				"nonce":       nonce,
				"signature":   w.Sign(nonce),
				"chainPubKey": map[string]string{"pubKey": w.address, "chain": "Ethereum"},
			},
		}),
	)
	require.Empty(t, response.CreateUser.Message)

	return response.CreateUser.Viewer.User.Dbid, username, response.CreateUser.Viewer.User.Galleries[0].Dbid
}

// newJWT generates a JWT
func newJWT(t *testing.T, userID persist.DBID) string {
	jwt, err := auth.JWTGeneratePipeline(context.Background(), userID)
	require.NoError(t, err)
	return jwt
}

// syncTokens makes a GraphQL request to sync a user's wallet
func syncTokens(t *testing.T, handler http.Handler, userID persist.DBID, address string) []persist.DBID {
	t.Helper()
	c := newClient(handler)
	var response = struct {
		SyncTokens struct {
			errMessage
			Viewer struct {
				User struct {
					Tokens []struct {
						Chain   string
						DBID    persist.DBID
						TokenID string
					}
				}
			}
		}
	}{}

	post(t, c, ops.Op("syncTokensMutation"), &response,
		withJWT(newJWT(t, userID)),
		client.Var("walletIds", []map[string]string{
			{"address": address, "chain": "Ethereum"},
		}),
	)
	require.Empty(t, response.SyncTokens.Message)

	tokens := make([]persist.DBID, len(response.SyncTokens.Viewer.User.Tokens))
	for i, token := range response.SyncTokens.Viewer.User.Tokens {
		tokens[i] = token.DBID
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

type operations map[string]string

// Op returns the named operation and fails if the operation does not exist
func (o operations) Op(name string) string {
	op, ok := o[name]
	if !ok {
		panic(fmt.Sprintf("`%s` does not exist", name))
	}
	return op
}

func loadOperations(filePath string) operations {
	ops, err := readOperationsFromFile(loadGeneratedSchema(), filePath)
	if err != nil {
		panic(err)
	}
	return ops
}

// readOperationsFromFile reads in a file of named GraphQL operations, validates them against a schema,
// and returns a mapping of operation names to operations. All GraphQL operations in the file must have names.
func readOperationsFromFile(schema *ast.Schema, filePath string) (operations, error) {
	ops := make(map[string]string)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	parsed, gqlErr := gqlparser.LoadQuery(schema, string(data))
	if gqlErr != nil {
		return nil, gqlErr
	}

	lastOpIndex := len(parsed.Operations) - 1

	for i, op := range parsed.Operations {
		if op.Name == "" {
			return nil, fmt.Errorf("error parsing file '%s': all GraphQL operations used in tests must have names", filePath)
		}

		position := op.Position
		opStart := op.Position.Start

		// A QueryDocument doesn't have an explicit way to get the entire source string for
		// a given operation, but we can assume that an operation extends from its own starting
		// position to the start of the next operation (or the end of the source if this is the
		// last operation)
		var opString string
		if i == lastOpIndex {
			opString = position.Src.Input[opStart:]
		} else {
			nextOp := parsed.Operations[i+1]
			opString = position.Src.Input[opStart:nextOp.Position.Start]
		}

		// The above method of getting a query string may include unnecessary leading/trailing
		// whitespace, so we'll get rid of it here to keep our operations consistent
		ops[op.Name] = strings.TrimSpace(opString)
	}

	return ops, nil
}

// loadGeneratedSchema loads the Gallery GraphQL schema via generated code
func loadGeneratedSchema() *ast.Schema {
	return generated.NewExecutableSchema(generated.Config{}).Schema()
}

// readCookie finds a cookie set in the response
func readCookie(t *testing.T, r *http.Response, name string) string {
	t.Helper()
	for _, c := range r.Cookies() {
		if c.Name == name {
			return c.Value
		}
	}
	require.NoError(t, fmt.Errorf("%s not set as a cookie", name))
	return ""
}

// withJWT adds a JWT to a gqlgen client request
func withJWT(jwt string) func(*client.Request) {
	return func(r *client.Request) {
		addJWT(r.HTTP, jwt)
	}
}

// addJWT adds a JWT to a HTTP request
func addJWT(r *http.Request, jwt string) {
	r.AddCookie(&http.Cookie{Name: auth.JWTCookieKey, Value: jwt})
}
