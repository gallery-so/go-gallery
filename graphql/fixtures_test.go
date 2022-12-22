package graphql_test

import (
	"os"
	"strings"
	"testing"

	migrate "github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/docker"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/stretchr/testify/require"
)

// fixture runs setup and teardown for a test
type fixture func(t *testing.T)

// withSetup sets up each fixture before running the test
func withSetup(test func(t *testing.T), fixtures ...fixture) func(t *testing.T) {
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

// fixturer defers running a fixture until setup is called
type fixturer interface {
	setup(t *testing.T)
}

// newNonceFixture generates a new nonce
type newNonceFixture struct {
	wallet wallet
	nonce  string
}

func (f *newNonceFixture) setup(t *testing.T) {
	t.Helper()
	wallet := newWallet(t)
	c := defaultClient()
	nonce := newNonce(t, c, wallet)

	f.wallet = wallet
	f.nonce = nonce
}

// newUserFixture generates a new user
type newUserFixture struct {
	wallet   wallet
	username string
	id       persist.DBID
}

func (f *newUserFixture) setup(t *testing.T) {
	t.Helper()
	wallet := newWallet(t)
	c := defaultClient()
	id, username := newUser(t, c, wallet)

	f.wallet = wallet
	f.username = username
	f.id = id
}
