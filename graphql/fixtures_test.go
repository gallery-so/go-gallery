package graphql_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	migrate "github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/docker"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/stretchr/testify/require"
)

// fixture runs setup and teardown for a test
type fixture func(t *testing.T)

// withFixtures sets up each fixture before running the test
func withFixtures(test func(t *testing.T), fixtures ...fixture) func(t *testing.T) {
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

// useTokenQueue is a fixture that creates a dummy queue for token processing.
func useTokenQueue(t *testing.T) {
	t.Helper()
	useCloudTasks(t)
	ctx := context.Background()
	client := task.NewClient(ctx)
	defer client.Close()
	queue, err := client.CreateQueue(ctx, &cloudtaskspb.CreateQueueRequest{
		Parent: "projects/gallery-test/locations/here",
		Queue: &cloudtaskspb.Queue{
			Name: "projects/gallery-test/locations/here/queues/token-processing",
		},
	})
	require.NoError(t, err)
	t.Setenv("TOKEN_PROCESSING_QUEUE", queue.Name)
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
	wallet    wallet
	username  string
	id        persist.DBID
	galleryID persist.DBID
}

func (f *newUserFixture) setup(t *testing.T) {
	t.Helper()
	wallet := newWallet(t)
	c := defaultClient()
	userID, username, galleryID := newUser(t, c, wallet)
	f.wallet = wallet
	f.username = username
	f.id = userID
	f.galleryID = galleryID
}

// newUserWithTokensFixture generates a new user with tokens synced
type newUserWithTokensFixture struct {
	newUserFixture
	tokenIDs []persist.DBID
}

func (f *newUserWithTokensFixture) setup(t *testing.T) {
	t.Helper()
	f.newUserFixture.setup(t)
	c := server.ClientInit(context.Background())
	p := multichain.Provider{
		Repos:       c.Repos,
		TasksClient: c.TaskClient,
		Queries:     c.Queries,
		Chains:      map[persist.Chain][]interface{}{persist.ChainETH: {&stubProvider{}}},
	}
	h := server.CoreInit(c, &p)
	f.tokenIDs = syncTokens(t, h, f.id, f.wallet.address)
}

// stubProvider returns the same response for every call
type stubProvider struct{}

func (p *stubProvider) GetTokensByWalletAddress(ctx context.Context, address persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	contract := multichain.ChainAgnosticContract{
		Address: "0x123",
		Name:    "testContract",
		Symbol:  "TEST",
	}

	tokens := []multichain.ChainAgnosticToken{}

	for i := 0; i < 10; i++ {
		tokens = append(tokens, multichain.ChainAgnosticToken{
			Name:            fmt.Sprintf("testToken%d", i),
			TokenID:         persist.TokenID(fmt.Sprintf("%X", i)),
			Quantity:        "1",
			ContractAddress: contract.Address,
			OwnerAddress:    address,
		})
	}

	return tokens, []multichain.ChainAgnosticContract{contract}, nil
}

func (p *stubProvider) GetTokensByContractAddress(ctx context.Context, contract persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("not implemented")
}

func (p *stubProvider) GetTokensByContractAddressAndOwner(ctx context.Context, owner persist.Address, contract persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("not implemented")
}

func (p *stubProvider) GetTokensByTokenIdentifiersAndOwner(context.Context, multichain.ChainAgnosticIdentifiers, persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("not implemented")
}
