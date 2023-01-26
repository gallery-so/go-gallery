//go:generate go run github.com/Khan/genqlient
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
	"strings"
	"testing"

	"github.com/Khan/genqlient/graphql"
	genql "github.com/Khan/genqlient/graphql"

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
			fixtures: []fixture{useDefaultEnv, usePostgres, useRedis, useTokenQueue, useNotificationTopics},
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
		{title: "views from multiple users are rolled up", run: testViewsAreRolledUp},
		{title: "should get trending users", run: testTrendingUsers, fixtures: []fixture{usePostgres, useRedis}},
		{title: "should get trending feed events", run: testTrendingFeedEvents},
		{title: "should update user experiences", run: testUpdateUserExperiences},
	}
	for _, test := range tests {
		t.Run(test.title, testWithFixtures(test.run, test.fixtures...))
	}
}

func testCreateUser(t *testing.T) {
	nonceF := newNonceFixture(t)
	c := defaultHandlerClient(t)
	username := "user" + persist.GenerateID().String()

	response, err := createUserMutation(context.Background(), c, authMechanismInput(nonceF.wallet, nonceF.nonce),
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
	response, err := userByUsernameQuery(context.Background(), defaultHandlerClient(t), userF.username)

	require.NoError(t, err)
	payload, _ := (*response.UserByUsername).(*userByUsernameQueryUserByUsernameGalleryUser)
	assert.Equal(t, userF.username, *payload.Username)
	assert.Equal(t, userF.id, payload.Dbid)
}

func testUserByAddress(t *testing.T) {
	userF := newUserFixture(t)
	c := authedHandlerClient(t, userF.id)

	response, err := userByAddressQuery(context.Background(), c, chainAddressInput(userF.wallet.address))

	require.NoError(t, err)
	payload, _ := (*response.UserByAddress).(*userByAddressQueryUserByAddressGalleryUser)
	assert.Equal(t, userF.username, *payload.Username)
	assert.Equal(t, userF.id, payload.Dbid)
}

func testUserByID(t *testing.T) {
	userF := newUserFixture(t)
	response, err := userByIdQuery(context.Background(), defaultHandlerClient(t), userF.id)

	require.NoError(t, err)
	payload, _ := (*response.UserById).(*userByIdQueryUserByIdGalleryUser)
	assert.Equal(t, userF.username, *payload.Username)
	assert.Equal(t, userF.id, payload.Dbid)
}

func testViewer(t *testing.T) {
	userF := newUserFixture(t)
	c := authedHandlerClient(t, userF.id)

	response, err := viewerQuery(context.Background(), c)

	require.NoError(t, err)
	payload, _ := (*response.Viewer).(*viewerQueryViewer)
	assert.Equal(t, userF.username, *payload.User.Username)
}

func testAddWallet(t *testing.T) {
	userF := newUserFixture(t)
	walletToAdd := newWallet(t)
	ctx := context.Background()
	c := authedHandlerClient(t, userF.id)
	nonce := newNonce(t, ctx, c, walletToAdd)

	response, err := addUserWalletMutation(ctx, c, chainAddressInput(walletToAdd.address), authMechanismInput(walletToAdd, nonce))

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
	ctx := context.Background()
	c := authedHandlerClient(t, userF.id)
	nonce := newNonce(t, ctx, c, walletToRemove)
	addResponse, err := addUserWalletMutation(ctx, c, chainAddressInput(walletToRemove.address), authMechanismInput(walletToRemove, nonce))
	require.NoError(t, err)
	wallets := (*addResponse.AddUserWallet).(*addUserWalletMutationAddUserWalletAddUserWalletPayload).Viewer.User.Wallets
	lastWallet := wallets[len(wallets)-1]
	assert.Len(t, wallets, 2)

	removeResponse, err := removeUserWalletsMutation(ctx, c, []persist.DBID{lastWallet.Dbid})

	require.NoError(t, err)
	payload, _ := (*removeResponse.RemoveUserWallets).(*removeUserWalletsMutationRemoveUserWalletsRemoveUserWalletsPayload)
	assert.Len(t, payload.Viewer.User.Wallets, 1)
	assert.NotEqual(t, lastWallet.Dbid, payload.Viewer.User.Wallets[0].Dbid)
}

func testLogin(t *testing.T) {
	userF := newUserFixture(t)
	ctx := context.Background()
	c := defaultHandlerClient(t)
	nonce := newNonce(t, ctx, c, userF.wallet)

	response, err := loginMutation(ctx, c, authMechanismInput(userF.wallet, nonce))

	require.NoError(t, err)
	payload, _ := (*response.Login).(*loginMutationLoginLoginPayload)
	assert.NotEmpty(t, readCookie(t, c.response, auth.JWTCookieKey))
	assert.Equal(t, userF.username, *payload.Viewer.User.Username)
	assert.Equal(t, userF.id, payload.Viewer.User.Dbid)
}

func testLogout(t *testing.T) {
	userF := newUserFixture(t)
	c := authedHandlerClient(t, userF.id)

	response, err := logoutMutation(context.Background(), c)

	require.NoError(t, err)
	assert.Empty(t, readCookie(t, c.response, auth.JWTCookieKey))
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
	c := customHandlerClient(t, h, withJWTOpt(t, userF.id))

	response, err := syncTokensMutation(context.Background(), c, []Chain{ChainEthereum})

	require.NoError(t, err)
	payload := (*response.SyncTokens).(*syncTokensMutationSyncTokensSyncTokensPayload)
	assert.NotEmpty(t, payload.Viewer.User.Tokens)
}

func testCreateCollection(t *testing.T) {
	userF := newUserWithTokensFixture(t)
	c := authedHandlerClient(t, userF.id)

	response, err := createCollectionMutation(context.Background(), c, CreateCollectionInput{
		GalleryId:      userF.galleryID,
		Name:           "newCollection",
		CollectorsNote: "this is a note",
		Tokens:         userF.tokenIDs,
		Layout:         defaultLayout(),
		TokenSettings:  defaultTokenSettings(userF.tokenIDs),
		Caption:        nil,
	})

	require.NoError(t, err)
	payload := (*response.CreateCollection).(*createCollectionMutationCreateCollectionCreateCollectionPayload)
	assert.NotEmpty(t, payload.Collection.Dbid)
	assert.Len(t, payload.Collection.Tokens, len(userF.tokenIDs))
}

func testUpdateUserExperiences(t *testing.T) {
	userF := newUserFixture(t)
	c := authedHandlerClient(t, userF.id)

	response, err := updateUserExperience(context.Background(), c, UpdateUserExperienceInput{
		ExperienceType: UserExperienceTypeMultigalleryannouncement,
		Experienced:    true,
	})

	require.NoError(t, err)
	bs, _ := json.Marshal(response)
	require.NotNil(t, response.UpdateUserExperience, string(bs))
	payload := (*response.UpdateUserExperience).(*updateUserExperienceUpdateUserExperienceUpdateUserExperiencePayload)
	assert.NotEmpty(t, payload.Viewer.UserExperiences)
	for _, experience := range payload.Viewer.UserExperiences {
		if experience.Type == UserExperienceTypeMultigalleryannouncement {
			assert.True(t, experience.Experienced)
		}
	}
}

func testViewsAreRolledUp(t *testing.T) {
	serverF := newServerFixture(t)
	userF := newUserFixture(t)
	bob := newUserFixture(t)
	alice := newUserFixture(t)
	ctx := context.Background()
	// bob views gallery
	client := authedServerClient(t, serverF.server.URL, bob.id)
	viewGallery(t, ctx, client, userF.galleryID)
	// // alice views gallery
	client = authedServerClient(t, serverF.server.URL, alice.id)
	viewGallery(t, ctx, client, userF.galleryID)

	// TODO: Actually verify that the views get rolled up
}

func testTrendingUsers(t *testing.T) {
	serverF := newServerFixture(t)
	bob := newUserFixture(t)
	alice := newUserFixture(t)
	dave := newUserFixture(t)
	ctx := context.Background()
	c := defaultServerClient(t, serverF.server.URL)
	// view bob a few times
	for i := 0; i < 5; i++ {
		viewGallery(t, ctx, c, bob.galleryID)
	}
	// view alice a few times
	for i := 0; i < 3; i++ {
		viewGallery(t, ctx, c, alice.galleryID)
	}
	// view dave a few times
	for i := 0; i < 1; i++ {
		viewGallery(t, ctx, c, dave.galleryID)
	}
	expected := []persist.DBID{bob.id, alice.id, dave.id}
	getTrending := func(t *testing.T, report ReportWindow) []persist.DBID {
		resp, err := trendingUsersQuery(ctx, c, TrendingUsersInput{Report: report})
		require.NoError(t, err)
		users := (*resp.GetTrendingUsers()).(*trendingUsersQueryTrendingUsersTrendingUsersPayload).GetUsers()
		actual := make([]persist.DBID, len(users))
		for i, u := range users {
			actual[i] = u.Dbid
		}
		return actual
	}

	t.Run("should pull the last 7 days", func(t *testing.T) {
		actual := getTrending(t, "LAST_7_DAYS")
		assert.EqualValues(t, expected, actual)
	})

	t.Run("should pull all time", func(t *testing.T) {
		actual := getTrending(t, "ALL_TIME")
		assert.EqualValues(t, expected, actual)
	})
}

func testTrendingFeedEvents(t *testing.T) {
	ctx := context.Background()
	userF := newUserWithFeedEventsFixture(t)
	c := authedHandlerClient(t, userF.id)
	admireFeedEvent(t, ctx, c, userF.feedEventIDs[1])
	commentOnFeedEvent(t, ctx, c, userF.feedEventIDs[1], "a")
	commentOnFeedEvent(t, ctx, c, userF.feedEventIDs[1], "b")
	commentOnFeedEvent(t, ctx, c, userF.feedEventIDs[1], "c")
	admireFeedEvent(t, ctx, c, userF.feedEventIDs[0])
	commentOnFeedEvent(t, ctx, c, userF.feedEventIDs[0], "a")
	commentOnFeedEvent(t, ctx, c, userF.feedEventIDs[0], "b")
	admireFeedEvent(t, ctx, c, userF.feedEventIDs[2])
	expected := []persist.DBID{userF.feedEventIDs[1], userF.feedEventIDs[0], userF.feedEventIDs[2]}

	actual := trendingFeedEvents(t, ctx, c, 10)

	assert.Equal(t, expected, actual)
}

// authMechanismInput signs a nonce with an ethereum wallet
func authMechanismInput(w wallet, nonce string) AuthMechanism {
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

func chainAddressInput(address string) ChainAddressInput {
	return ChainAddressInput{Address: address, Chain: "Ethereum"}
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

func newNonce(t *testing.T, ctx context.Context, c graphql.Client, w wallet) string {
	t.Helper()
	response, err := getAuthNonceMutation(ctx, c, chainAddressInput(w.address))
	require.NoError(t, err)
	payload := (*response.GetAuthNonce).(*getAuthNonceMutationGetAuthNonce)
	return *payload.Nonce
}

// newUser makes a GraphQL request to generate a new user
func newUser(t *testing.T, ctx context.Context, c graphql.Client, w wallet) (userID persist.DBID, username string, galleryID persist.DBID) {
	t.Helper()
	nonce := newNonce(t, ctx, c, w)
	username = "user" + persist.GenerateID().String()

	response, err := createUserMutation(ctx, c, authMechanismInput(w, nonce),
		CreateUserInput{Username: username},
	)

	require.NoError(t, err)
	payload := (*response.CreateUser).(*createUserMutationCreateUserCreateUserPayload)
	return payload.Viewer.User.Dbid, username, payload.Viewer.User.Galleries[0].Dbid
}

// newJWT generates a JWT
func newJWT(t *testing.T, ctx context.Context, userID persist.DBID) string {
	jwt, err := auth.JWTGeneratePipeline(ctx, userID)
	require.NoError(t, err)
	return jwt
}

// syncTokens makes a GraphQL request to sync a user's wallet
func syncTokens(t *testing.T, ctx context.Context, c graphql.Client, userID persist.DBID) []persist.DBID {
	t.Helper()
	resp, err := syncTokensMutation(ctx, c, []Chain{"Ethereum"})
	require.NoError(t, err)
	payload := (*resp.SyncTokens).(*syncTokensMutationSyncTokensSyncTokensPayload)
	tokens := make([]persist.DBID, len(payload.Viewer.User.Tokens))
	for i, token := range payload.Viewer.User.Tokens {
		tokens[i] = token.Dbid
	}
	return tokens
}

// viewGallery makes a GraphQL request to view a gallery
func viewGallery(t *testing.T, ctx context.Context, c graphql.Client, galleryID persist.DBID) {
	t.Helper()
	resp, err := viewGalleryMutation(ctx, c, galleryID)
	require.NoError(t, err)
	_ = (*resp.ViewGallery).(*viewGalleryMutationViewGalleryViewGalleryPayload)
}

// createCollection makes a GraphQL request to create a collection
func createCollection(t *testing.T, ctx context.Context, c graphql.Client, input CreateCollectionInput) persist.DBID {
	t.Helper()
	resp, err := createCollectionMutation(ctx, c, input)
	require.NoError(t, err)
	payload := (*resp.CreateCollection).(*createCollectionMutationCreateCollectionCreateCollectionPayload)
	return payload.Collection.Dbid
}

// globalFeedEvents makes a GraphQL request to return existing feed events
func globalFeedEvents(t *testing.T, ctx context.Context, c graphql.Client, limit int) []persist.DBID {
	t.Helper()
	resp, err := globalFeedQuery(ctx, c, &limit)
	require.NoError(t, err)
	feedEvents := make([]persist.DBID, len(resp.GlobalFeed.Edges))
	for i, event := range resp.GlobalFeed.Edges {
		e := (*event.Node).(*globalFeedQueryGlobalFeedFeedConnectionEdgesFeedEdgeNodeFeedEvent)
		feedEvents[i] = e.Dbid

	}
	return feedEvents
}

// trendingFeedEvents makes a GraphQL request to return trending feedEvents
func trendingFeedEvents(t *testing.T, ctx context.Context, c graphql.Client, limit int) []persist.DBID {
	t.Helper()
	resp, err := trendingFeedQuery(ctx, c, &limit)
	require.NoError(t, err)
	feedEvents := make([]persist.DBID, len(resp.TrendingFeed.Edges))
	for i, event := range resp.TrendingFeed.Edges {
		e := (*event.Node).(*trendingFeedQueryTrendingFeedFeedConnectionEdgesFeedEdgeNodeFeedEvent)
		feedEvents[i] = e.Dbid

	}
	return feedEvents
}

// admireFeedEvent makes a GraphQL request to admire a feed event
func admireFeedEvent(t *testing.T, ctx context.Context, c graphql.Client, feedEventID persist.DBID) {
	t.Helper()
	resp, err := admireFeedEventMutation(ctx, c, feedEventID)
	require.NoError(t, err)
	_ = (*resp.AdmireFeedEvent).(*admireFeedEventMutationAdmireFeedEventAdmireFeedEventPayload)
}

// commentOnFeedEvent makes a GraphQL request to admire a feed event
func commentOnFeedEvent(t *testing.T, ctx context.Context, c graphql.Client, feedEventID persist.DBID, comment string) {
	t.Helper()
	resp, err := commentOnFeedEventMutation(ctx, c, feedEventID, comment)
	require.NoError(t, err)
	_ = (*resp.CommentOnFeedEvent).(*commentOnFeedEventMutationCommentOnFeedEventCommentOnFeedEventPayload)
}

// defaultLayout returns a collection layout of one section with one column
func defaultLayout() CollectionLayoutInput {
	return CollectionLayoutInput{
		Sections: []int{0},
		SectionLayout: []CollectionSectionLayoutInput{
			{
				Columns:    0,
				Whitespace: []int{},
			},
		},
	}
}

// defaultTokenSettings returns default display token settings
func defaultTokenSettings(tokens []persist.DBID) []CollectionTokenSettingsInput {
	settings := make([]CollectionTokenSettingsInput, len(tokens))
	for i, token := range tokens {
		settings[i] = CollectionTokenSettingsInput{TokenId: token}
	}
	return settings
}

// defaultHandler returns a backend GraphQL http.Handler
func defaultHandler() http.Handler {
	c := server.ClientInit(context.Background())
	p := server.NewMultichainProvider(c)
	handler := server.CoreInit(c, p)
	return handler
}

// defaultHandlerClient returns a GraphQL client attached to a backend GraphQL handler
func defaultHandlerClient(t *testing.T) *handlerClient {
	return customHandlerClient(t, defaultHandler())
}

// authedHandlerClient returns a GraphQL client with an authenticated JWT
func authedHandlerClient(t *testing.T, userID persist.DBID) *handlerClient {
	return customHandlerClient(t, defaultHandler(), withJWTOpt(t, userID))
}

// customHandlerClient configures the client with the provided HTTP handler and client options
func customHandlerClient(t *testing.T, handler http.Handler, opts ...func(*http.Request)) *handlerClient {
	return &handlerClient{handler: handler, opts: opts, endpoint: "/glry/graphql/query"}
}

// defaultServerClient provides a client to a live server
func defaultServerClient(t *testing.T, host string) *serverClient {
	return customServerClient(t, host)
}

// authedServerClient provides an authenticated client to a live server
func authedServerClient(t *testing.T, host string, userID persist.DBID) *serverClient {
	return customServerClient(t, host, withJWTOpt(t, userID))
}

// customServerClient provides a client to a live server with custom options
func customServerClient(t *testing.T, host string, opts ...func(*http.Request)) *serverClient {
	return &serverClient{url: host + "/glry/graphql/query", opts: opts}
}

// withJWTOpt ddds a JWT cookie to the request headers
func withJWTOpt(t *testing.T, userID persist.DBID) func(*http.Request) {
	jwt, err := auth.JWTGeneratePipeline(context.Background(), userID)
	require.NoError(t, err)
	return func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: auth.JWTCookieKey, Value: jwt})
	}
}

// handlerClient records the server response for testing purposes
type handlerClient struct {
	handler  http.Handler
	endpoint string
	opts     []func(r *http.Request)
	response *http.Response
}

func (c *handlerClient) MakeRequest(ctx context.Context, req *genql.Request, resp *genql.Response) error {
	body, err := json.Marshal(map[string]any{
		"query":     req.Query,
		"variables": req.Variables,
	})
	if err != nil {
		return err
	}

	r := httptest.NewRequest(http.MethodPost, c.endpoint, io.NopCloser(bytes.NewBuffer(body)))
	r.Header.Set("Content-Type", "application/json")
	r.URL.Path = c.endpoint
	for _, opt := range c.opts {
		opt(r)
	}

	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, r)

	res := w.Result()
	c.response = res
	defer res.Body.Close()

	return json.Unmarshal(w.Body.Bytes(), resp)
}

// serverClient makes a request to a running server
type serverClient struct {
	url      string
	opts     []func(r *http.Request)
	response *http.Response
}

func (c *serverClient) MakeRequest(ctx context.Context, req *genql.Request, resp *genql.Response) error {
	body, err := json.Marshal(map[string]any{
		"query":     req.Query,
		"variables": req.Variables,
	})
	if err != nil {
		return err
	}

	r := httptest.NewRequest(http.MethodPost, c.url, io.NopCloser(bytes.NewBuffer(body)))
	r.Header.Set("Content-Type", "application/json")
	r.RequestURI = ""
	for _, opt := range c.opts {
		opt(r)
	}

	res, err := http.DefaultClient.Do(r)
	if err != nil {
		return err
	}
	c.response = res
	defer res.Body.Close()

	return json.NewDecoder(res.Body).Decode(resp)
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
