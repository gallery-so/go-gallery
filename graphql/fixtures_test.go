package graphql_test

import (
	"context"
	"fmt"
	"net/http/httptest"
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
	"github.com/mikeydub/go-gallery/service/pubsub/gcp"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/require"
)

// fixture runs setup and teardown for a test
type fixture func(t *testing.T)

// testWithFixtures sets up each fixture before running the test
func testWithFixtures(test func(t *testing.T), fixtures ...fixture) func(t *testing.T) {
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
// when the test and its subtests complete
func useRedis(t *testing.T) {
	t.Helper()
	r, err := docker.StartRedis()
	require.NoError(t, err)
	t.Setenv("REDIS_URL", r.GetHostPort("6379/tcp"))
	t.Cleanup(func() { r.Close() })
}

// useTokenQueue is a fixture that creates a dummy queue for token processing
func useTokenQueue(t *testing.T) {
	t.Helper()
	useCloudTasks(t)
	ctx := context.Background()
	client := task.NewClient(ctx)
	defer client.Close()
	queue, err := client.CreateQueue(ctx, &cloudtaskspb.CreateQueueRequest{
		Parent: "projects/gallery-test/locations/here",
		Queue: &cloudtaskspb.Queue{
			Name: "projects/gallery-test/locations/here/queues/token-processing-" + persist.GenerateID().String(),
		},
	})
	require.NoError(t, err)
	t.Setenv("TOKEN_PROCESSING_QUEUE", queue.Name)
}

// useNotificationTopics is a fixture that creates dummy PubSub topics for notifications
func useNotificationTopics(t *testing.T) {
	t.Helper()
	usePubSub(t)
	ctx := context.Background()
	client := gcp.NewClient(ctx)

	newNotificationsTopic := "new-notifications" + persist.GenerateID().String()
	_, err := client.CreateTopic(ctx, newNotificationsTopic)
	require.NoError(t, err)
	t.Setenv("PUBSUB_TOPIC_NEW_NOTIFICATIONS", newNotificationsTopic)

	updatedNotificationsTopic := "updated-notifications" + persist.GenerateID().String()
	_, err = client.CreateTopic(ctx, updatedNotificationsTopic)
	require.NoError(t, err)
	t.Setenv("PUBSUB_TOPIC_UPDATED_NOTIFICATIONS", updatedNotificationsTopic)
}

// useCloudTasks starts a running Cloud Tasks emulator
func useCloudTasks(t *testing.T) {
	t.Helper()
	r, err := docker.StartCloudTasks()
	require.NoError(t, err)
	t.Setenv("TASK_QUEUE_HOST", r.GetHostPort("8123/tcp"))
	t.Cleanup(func() { r.Close() })
}

// usePubSub starts a running PubSub emulator
func usePubSub(t *testing.T) {
	t.Helper()
	r, err := docker.StartPubSub()
	require.NoError(t, err)
	t.Setenv("PUBSUB_EMULATOR_HOST", r.GetHostPort("8085/tcp"))
	t.Cleanup(func() { r.Close() })
}

type serverFixture struct {
	server *httptest.Server
}

// newServerFixture starts a new HTTP server for end-to-end tests
func newServerFixture(t *testing.T) serverFixture {
	t.Helper()
	server := httptest.NewServer(defaultHandler())
	t.Cleanup(func() { server.Close() })
	return serverFixture{server}
}

type nonceFixture struct {
	wallet wallet
	nonce  string
}

// newNonceFixture generates a new nonce
func newNonceFixture(t *testing.T) nonceFixture {
	t.Helper()
	wallet := newWallet(t)
	ctx := context.Background()
	c := defaultHandlerClient(t)
	nonce := newNonce(t, ctx, c, wallet)
	return nonceFixture{wallet, nonce}
}

type userFixture struct {
	wallet    wallet
	username  string
	id        persist.DBID
	galleryID persist.DBID
}

// newUserFixture generates a new user
func newUserFixture(t *testing.T) userFixture {
	t.Helper()
	wallet := newWallet(t)
	ctx := context.Background()
	c := defaultHandlerClient(t)
	userID, username, galleryID := newUser(t, ctx, c, wallet)
	return userFixture{wallet, username, userID, galleryID}
}

type userWithTokensFixture struct {
	userFixture
	tokenIDs []persist.DBID
}

// newUserWithTokensFixture generates a new user with tokens synced
func newUserWithTokensFixture(t *testing.T) userWithTokensFixture {
	t.Helper()
	user := newUserFixture(t)
	ctx := context.Background()
	clients := server.ClientInit(ctx)
	p := multichain.Provider{
		Repos:       clients.Repos,
		TasksClient: clients.TaskClient,
		Queries:     clients.Queries,
		Chains:      map[persist.Chain][]interface{}{persist.ChainETH: {&stubProvider{}}},
	}
	h := server.CoreInit(clients, &p)
	c := customHandlerClient(t, h, withJWTOpt(t, user.id))
	tokenIDs := syncTokens(t, ctx, c, user.id)
	return userWithTokensFixture{user, tokenIDs}
}

type userWithFeedEventsFixture struct {
	userWithTokensFixture
	feedEventIDs []persist.DBID
}

// newUserWithFeedEventsFixture generates a new user with feed events pre-generated
func newUserWithFeedEventsFixture(t *testing.T) userWithFeedEventsFixture {
	t.Helper()
	serverF := newServerFixture(t)
	user := newUserWithTokensFixture(t)
	ctx := context.Background()
	c := authedServerClient(t, serverF.server.URL, user.id)
	// At the moment, we rely on captioning to ensure that that feed events are
	// generated near instantly so that we don't have to add arbitrary sleep
	// times during tests.
	createCollection(t, ctx, c, CreateCollectionInput{
		GalleryId:     user.galleryID,
		Tokens:        user.tokenIDs,
		Layout:        defaultLayout(),
		TokenSettings: defaultTokenSettings(user.tokenIDs),
		Caption:       util.StringToPointer("this is a caption"),
	})
	createCollection(t, ctx, c, CreateCollectionInput{
		GalleryId:     user.galleryID,
		Tokens:        user.tokenIDs,
		Layout:        defaultLayout(),
		TokenSettings: defaultTokenSettings(user.tokenIDs),
		Caption:       util.StringToPointer("this is a caption"),
	})
	createCollection(t, ctx, c, CreateCollectionInput{
		GalleryId:     user.galleryID,
		Tokens:        user.tokenIDs,
		Layout:        defaultLayout(),
		TokenSettings: defaultTokenSettings(user.tokenIDs),
		Caption:       util.StringToPointer("this is a caption"),
	})
	feedEvents := globalFeedEvents(t, ctx, c, 3)
	require.Len(t, feedEvents, 3)
	return userWithFeedEventsFixture{user, feedEvents}
}

// stubProvider returns the same response for every call made to it
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
