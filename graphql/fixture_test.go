package graphql_test

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	migrate "github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/docker"
	"github.com/mikeydub/go-gallery/graphql/dummymetadata"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/pubsub/gcp"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/tokenprocessing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/require"
)

// fixture runs setup and teardown for a test
type fixture func(t *testing.T)

// testWithFixtures sets up each fixture before running the test
func testWithFixtures(test func(t *testing.T), fixtures ...fixture) func(t *testing.T) {
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
	hostAndPort := strings.Split(r.GetHostPort("5432/tcp"), ":")
	t.Setenv("POSTGRES_HOST", hostAndPort[0])
	t.Setenv("POSTGRES_PORT", hostAndPort[1])

	err = migrate.RunMigrations(postgres.MustCreateClient(postgres.WithUser("postgres")), "./db/migrations/core")
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

// useTokenProcessing starts a HTTP server for tokenprocessing
func useTokenProcessing(t *testing.T) {
	t.Helper()
	c := server.ClientInit(context.Background())
	p := server.NewMultichainProvider(c)
	server := httptest.NewServer(tokenprocessing.CoreInitServer(c, p))
	t.Setenv("TOKEN_PROCESSING_URL", server.URL)
	t.Cleanup(func() {
		server.Close()
		c.Close()
	})
}

type serverFixture struct {
	*httptest.Server
}

// newServerFixture starts a new HTTP server for end-to-end tests
func newServerFixture(t *testing.T) serverFixture {
	t.Helper()
	server := httptest.NewServer(defaultHandler(t))
	t.Cleanup(func() { server.Close() })
	return serverFixture{server}
}

// newMetadataServerFixture starts a HTTP server for fetching static metadata
func newMetadataServerFixture(t *testing.T) serverFixture {
	t.Helper()
	server := httptest.NewServer(dummymetadata.CoreInitServer())
	t.Cleanup(func() { server.Close() })
	return serverFixture{server}
}

type nonceFixture struct {
	Wallet wallet
	Nonce  string
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
	Wallet    wallet
	Username  string
	ID        persist.DBID
	GalleryID persist.DBID
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
	TokenIDs []persist.DBID
}

// newUserWithTokensFixture generates a new user with tokens synced
func newUserWithTokensFixture(t *testing.T) userWithTokensFixture {
	t.Helper()
	user := newUserFixture(t)
	ctx := context.Background()
	h := handlerWithProviders(t, sendTokensNOOP, defaultStubProvider(user.Wallet.Address))
	c := customHandlerClient(t, h, withJWTOpt(t, user.ID))
	tokenIDs := syncTokens(t, ctx, c, user.ID)
	return userWithTokensFixture{user, tokenIDs}
}

type userWithFeedEventsFixture struct {
	userWithTokensFixture
	FeedEventIDs []persist.DBID
}

// newUserWithFeedEventsFixture generates a new user with feed events pre-generated
func newUserWithFeedEventsFixture(t *testing.T) userWithFeedEventsFixture {
	t.Helper()
	serverF := newServerFixture(t)
	user := newUserWithTokensFixture(t)
	ctx := context.Background()
	c := authedServerClient(t, serverF.Server.URL, user.ID)
	// At the moment, we rely on captioning to ensure that that feed events are
	// generated near instantly so that we don't have to add arbitrary sleep
	// times during tests.
	createCollection(t, ctx, c, CreateCollectionInput{
		GalleryId:     user.GalleryID,
		Tokens:        user.TokenIDs,
		Layout:        defaultLayout(),
		TokenSettings: defaultTokenSettings(user.TokenIDs),
		Caption:       util.ToPointer("this is a caption"),
	})
	createCollection(t, ctx, c, CreateCollectionInput{
		GalleryId:     user.GalleryID,
		Tokens:        user.TokenIDs,
		Layout:        defaultLayout(),
		TokenSettings: defaultTokenSettings(user.TokenIDs),
		Caption:       util.ToPointer("this is a caption"),
	})
	createCollection(t, ctx, c, CreateCollectionInput{
		GalleryId:     user.GalleryID,
		Tokens:        user.TokenIDs,
		Layout:        defaultLayout(),
		TokenSettings: defaultTokenSettings(user.TokenIDs),
		Caption:       util.ToPointer("this is a caption"),
	})
	feedEvents := globalFeedEvents(t, ctx, c, 3)
	require.Len(t, feedEvents, 3)
	return userWithFeedEventsFixture{user, feedEvents}
}
