package graphql_test

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/ethereum/go-ethereum/crypto"
	migrate "github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/docker"
	"github.com/mikeydub/go-gallery/graphql/generated"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

var (
	operations = loadOperations()
)

type testCase struct {
	title    string
	run      func(t *testing.T)
	fixtures []fixture
}

func TestGraphQL(t *testing.T) {
	tests := []testCase{
		{
			title:    "test user API",
			run:      testAPI_User,
			fixtures: []fixture{useDefaultEnv, usePostgres},
		},
	}
	for _, test := range tests {
		t.Run(test.title, withFixtures(test.run, test.fixtures...))
	}
}

func testAPI_User(t *testing.T) {
	tests := []testCase{
		{title: "should get user by ID", run: testUserByID},
		{title: "should get user by username", run: testUserByUsername},
		{title: "should get user by address", run: testUserByAddress},
		{title: "should get viewer", run: testViewer},
	}
	for _, test := range tests {
		t.Run(test.title, test.run)
	}
}

func testUserByUsername(t *testing.T) {
	c := newClient()

	wallet := newWallet(t)
	id, username := newUser(t, c, wallet)

	var response = struct {
		UserByUsername struct {
			model.GalleryUser
			errMessage
		}
	}{}

	post(t, c, mustGet(operations, "userByUsernameQuery"), &response, client.Var("user", username))

	require.Empty(t, response.UserByUsername.Message)
	assert.Equal(t, username, *response.UserByUsername.Username)
	assert.Equal(t, id, response.UserByUsername.Dbid)
}

func testUserByAddress(t *testing.T) {
	c := newClient()
	wallet := newWallet(t)
	id, username := newUser(t, c, wallet)

	var response = struct {
		UserByAddress struct {
			model.GalleryUser
			errMessage
		}
	}{}

	post(t, c, mustGet(operations, "userByAddressQuery"), &response,
		client.Var("input", map[string]string{
			"address": wallet.address,
			"chain":   "Ethereum",
		}),
	)

	require.Empty(t, response.UserByAddress.Message)
	assert.Equal(t, username, *response.UserByAddress.Username)
	assert.Equal(t, id, response.UserByAddress.Dbid)
}

func testUserByID(t *testing.T) {
	c := newClient()
	wallet := newWallet(t)
	id, username := newUser(t, c, wallet)

	var response = struct {
		UserByID struct {
			model.GalleryUser
			errMessage
		}
	}{}

	post(t, c, mustGet(operations, "userByIdQuery"), &response, client.Var("id", id))

	require.Empty(t, response.UserByID.Message)
	assert.Equal(t, username, *response.UserByID.Username)
	assert.Equal(t, id, response.UserByID.Dbid)
}

func testViewer(t *testing.T) {
	c := newClient()
	wallet := newWallet(t)
	id, username := newUser(t, c, wallet)

	var response = struct {
		Viewer struct {
			model.Viewer
			errMessage
		}
	}{}

	post(t, c, mustGet(operations, "viewerQuery"), &response, func(r *client.Request) {
		jwt, err := auth.JWTGeneratePipeline(context.Background(), id)
		require.NoError(t, err)
		r.HTTP.AddCookie(&http.Cookie{
			Name:  auth.JWTCookieKey,
			Value: jwt,
		})
	})

	require.Empty(t, response.Viewer.Message)
	assert.Equal(t, username, *response.Viewer.User.Username)
}

// mustGet fails if the key does not exist in the map
func mustGet[V any](m map[string]V, key string) V {
	val, ok := m[key]
	if !ok {
		panic(fmt.Sprintf("`%s` does not exist in map", key))
	}
	return val
}

// errMessage represents a handled graphql error
type errMessage struct {
	Typename string `json:"__typename"`
	Message  string `json:"message"`
}

func post(t *testing.T, c *client.Client, query string, into any, options ...client.Option) {
	t.Helper()
	r, err := c.RawPost(query, options...)
	require.Empty(t, r.Errors)
	require.NoError(t, err)

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

type eoa struct {
	pKey    *ecdsa.PrivateKey
	pubKey  *ecdsa.PublicKey
	address string
}

func (e *eoa) Sign(msg string) string {
	sig, err := crypto.Sign(crypto.Keccak256([]byte(msg)), e.pKey)
	if err != nil {
		panic(err)
	}
	return "0x" + hex.EncodeToString(sig)
}

// newWallet generates a new wallet for testing purposes
func newWallet(t *testing.T) eoa {
	t.Helper()
	pk, err := crypto.GenerateKey()
	require.NoError(t, err)

	pubKey := pk.Public().(*ecdsa.PublicKey)
	address := crypto.PubkeyToAddress(*pubKey).Hex()

	return eoa{
		pKey:    pk,
		pubKey:  pubKey,
		address: address,
	}
}

// newClient returns a new graphql client
// Requests are made to the graphql route by default
func newClient() *client.Client {
	handler := server.CoreInit(postgres.NewClient(), postgres.NewPgxClient())
	return client.New(handler, func(bd *client.Request) {
		bd.HTTP.URL.Path = "/glry/graphql/query"
	})
}

type fixture func(t *testing.T)

// withFixtures sets up each fixture before running the test
func withFixtures(test func(t *testing.T), fixtures ...fixture) func(t *testing.T) {
	return func(t *testing.T) {
		for _, fixture := range fixtures {
			fixture(t)
		}
		test(t)
	}
}

// useDefaultEnv sets the test environment to the default server environment
func useDefaultEnv(t *testing.T) {
	prevValues := make(map[string]string)
	for _, envVar := range os.Environ() {
		kv := strings.Split(envVar, "=")
		prevValues[kv[0]] = kv[1]
	}

	server.SetDefaults()
	curValues := os.Environ()

	t.Cleanup(func() {
		for _, envVar := range curValues {
			k := strings.Split(envVar, "=")[0]
			if prevVal, ok := prevValues[k]; ok {
				os.Setenv(k, prevVal)
			} else {
				os.Unsetenv(k)
			}
		}
	})
}

// usePostgres starts a running Postgres Docker container with migrations applied.
// The passed testing.T arg resets the environment and deletes the container
// when the test and its subtests complete.
func usePostgres(t *testing.T) {
	t.Helper()
	r, err := docker.StartPostgres()
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	hostAndPort := strings.Split(r.GetHostPort("5432/tcp"), ":")
	t.Setenv("POSTGRES_HOST", hostAndPort[0])
	t.Setenv("POSTGRES_PORT", hostAndPort[1])
	err = migrate.RunMigration(postgres.NewClient(), "./db/migrations/core")
	require.NoError(t, err)
}

func newNonce(t *testing.T, c *client.Client, wallet eoa) string {
	t.Helper()
	response := struct {
		GetAuthNonce struct {
			model.AuthNonce
			errMessage
		}
	}{}

	post(t, c, mustGet(operations, "getAuthNonceMutation"), &response,
		client.Var("input", map[string]string{
			"address": wallet.address,
			"chain":   "Ethereum",
		}),
	)
	require.Empty(t, response.GetAuthNonce.Message)

	return *response.GetAuthNonce.Nonce
}

func newUser(t *testing.T, c *client.Client, wallet eoa) (persist.DBID, string) {
	t.Helper()
	nonce := newNonce(t, c, wallet)
	username := "user" + persist.GenerateID().String()

	var response = struct {
		CreateUser struct {
			Viewer model.Viewer
			errMessage
		}
	}{}

	post(t, c, mustGet(operations, "createUserMutation"), &response,
		client.Var("username", username),
		client.Var("authMethod", map[string]any{
			"eoa": map[string]any{
				"nonce":       nonce,
				"signature":   wallet.Sign(nonce),
				"chainPubKey": map[string]string{"pubKey": wallet.address, "chain": "Ethereum"},
			},
		}),
	)
	require.Empty(t, response.CreateUser.Message)

	return response.CreateUser.Viewer.User.Dbid, username
}

// loadOperations loads test queries from the testdata folder
func loadOperations() map[string]string {
	ops, err := readOperationsFromFile(loadGeneratedSchema(), util.MustFindFile("./testdata/operations.gql"))
	if err != nil {
		panic(err)
	}
	return ops
}

// readOperationsFromFile reads in a file of named GraphQL operations, validates them against a schema,
// and returns a mapping of operation names to operations. All GraphQL operations in the file must have names.
func readOperationsFromFile(schema *ast.Schema, filePath string) (map[string]string, error) {
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
