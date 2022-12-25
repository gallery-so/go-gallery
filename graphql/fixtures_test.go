package graphql_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	migrate "github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/docker"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/stretchr/testify/require"
)

// fixture runs setup and teardown for a test
type fixture func(t *testing.T)

// withSetup sets up each fixture before running the test
func withSetup(test func(t *testing.T), fixtures ...fixture) func(t *testing.T) {
	return func(t *testing.T) {
		var wg sync.WaitGroup
		for _, fixture := range fixtures {
			fixture := fixture
			wg.Add(1)
			go func() {
				defer wg.Done()
				fixture(t)
			}()
		}
		wg.Wait()
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
	hostAndPort := strings.Split(r.GetHostPort("5432/tcp"), ":")
	t.Setenv("POSTGRES_HOST", hostAndPort[0])
	t.Setenv("POSTGRES_PORT", hostAndPort[1])
	err = migrate.RunMigration(postgres.NewClient(), "./db/migrations/core")
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })
}

// useRedis starts a running Redis Docker container and stops the instance
// when the test and its subtests complete.
func useRedis(t *testing.T) {
	t.Helper()
	r, err := docker.StartRedis()
	require.NoError(t, err)
	t.Setenv("REDIS_URL", r.GetHostPort("6379/tcp"))
	t.Cleanup(func() { r.Close() })
}

// useCloudTasks starts a running Cloud Tasks emulator with a set of tasks queues created.
func useCloudTasks(t *testing.T) {
	t.Helper()
	r, err := docker.StartCloudTasks()
	require.NoError(t, err)
	t.Setenv("TASK_QUEUE_HOST", r.GetHostPort("8123/tcp"))
	t.Cleanup(func() { r.Close() })
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

// newUserWithTokensFixtures generates a new user with tokens synced
type newUserWithTokensFixtures struct {
	newUserFixture
	tokenIDs []persist.DBID
}

func (f *newUserWithTokensFixtures) setup(t *testing.T) {
	t.Helper()
	f.newUserFixture.setup(t)
	r := server.ResourcesInit(context.Background())
	p := multichain.Provider{
		Repos:       r.Repos,
		TasksClient: r.TaskClient,
		Queries:     r.Queries,
		Chains:      map[persist.Chain][]interface{}{persist.ChainETH: {&stubProvider{}}},
	}
	h := server.CoreInit(r, &p)
	f.tokenIDs = syncTokens(t, h, f.id, f.wallet.address)
}

type stubProvider struct{}

func (p *stubProvider) GetTokensByWalletAddress(ctx context.Context, address persist.Address, limit int, offset int) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	contract := multichain.ChainAgnosticContract{
		Address: "0x123",
		Name:    "testContract",
		Symbol:  "TEST",
	}

	tokens := []multichain.ChainAgnosticToken{}

	if limit == 0 {
		limit = 10
	}

	for i := 0; i < limit; i++ {
		tokenID := i * offset
		tokens = append(tokens, multichain.ChainAgnosticToken{
			Name:            fmt.Sprintf("testToken%d", tokenID),
			TokenID:         persist.TokenID(fmt.Sprintf("%X", tokenID)),
			Quantity:        "1",
			ContractAddress: contract.Address,
			OwnerAddress:    address,
		})
	}

	return tokens, []multichain.ChainAgnosticContract{contract}, nil
}

func (p *stubProvider) GetTokensByContractAddress(ctx context.Context, contract persist.Address, limit int, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("not implemented")
}

func (p *stubProvider) GetTokensByContractAddressAndOwner(ctx context.Context, owner persist.Address, contract persist.Address, limit int, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("not implemented")
}

func (p *stubProvider) GetTokensByTokenIdentifiersAndOwner(context.Context, multichain.ChainAgnosticIdentifiers, persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("not implemented")
}
