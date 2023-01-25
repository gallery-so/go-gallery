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
	"github.com/mikeydub/go-gallery/util"
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
		{title: "update gallery and create a feed event with a caption", run: testUpdateGalleryWithCaption},
		{title: "update gallery and ensure name still gets set when not sent in update", run: testUpdateGalleryWithNoNameChange},
		{title: "update gallery with a new collection", run: testUpdateGalleryWithNewCollection},
		{title: "should get trending users", run: testTrendingUsers, fixtures: []fixture{usePostgres, useRedis}},
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
	c := authedHandlerClient(t, userF.id)
	nonce := newNonce(t, c, walletToAdd)

	response, err := addUserWalletMutation(context.Background(), c, chainAddressInput(walletToAdd.address), authMechanismInput(walletToAdd, nonce))

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
	c := authedHandlerClient(t, userF.id)
	nonce := newNonce(t, c, walletToRemove)
	addResponse, err := addUserWalletMutation(context.Background(), c, chainAddressInput(walletToRemove.address), authMechanismInput(walletToRemove, nonce))
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
	c := defaultHandlerClient(t)
	nonce := newNonce(t, c, userF.wallet)

	response, err := loginMutation(context.Background(), c, authMechanismInput(userF.wallet, nonce))

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

func testUpdateGalleryWithCaption(t *testing.T) {
	userF := newUserWithTokensFixture(t)
	c := authedHandlerClient(t, userF.id)

	colResp, err := createCollectionMutation(context.Background(), c, CreateCollectionInput{
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
	colPay := (*colResp.CreateCollection).(*createCollectionMutationCreateCollectionCreateCollectionPayload)
	assert.NotEmpty(t, colPay.Collection.Dbid)
	assert.Len(t, colPay.Collection.Tokens, 1)

	response, err := updateGalleryMutation(context.Background(), c, UpdateGalleryInput{
		GalleryId: userF.galleryID,
		Name:      util.StringToPointer("newName"),
		UpdatedCollections: []*UpdateCollectionInput{
			{
				Dbid:           colPay.Collection.Dbid,
				Tokens:         userF.tokenIDs[:2],
				Name:           "yes",
				CollectorsNote: "no",
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
				}},
		},
		CreatedCollections: []*CreateCollectionInGalleryInput{
			{
				GivenID:        "wow",
				Tokens:         userF.tokenIDs[:3],
				CollectorsNote: "this is a note",
				Name:           "newCollection",
				Layout: CollectionLayoutInput{
					Sections: []int{0},
					SectionLayout: []CollectionSectionLayoutInput{
						{
							Columns:    3,
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
			},
		},
		Order:   []persist.DBID{colPay.Collection.Dbid, "wow"},
		Caption: util.StringToPointer("newCaption"),
	})

	require.NoError(t, err)
	require.NotNil(t, response.UpdateGallery)
	payload, ok := (*response.UpdateGallery).(*updateGalleryMutationUpdateGalleryUpdateGalleryPayload)
	if !ok {
		err := (*response.UpdateGallery).(*updateGalleryMutationUpdateGalleryErrInvalidInput)
		t.Fatal(err)
	}
	assert.NotEmpty(t, payload.Gallery.Name)
	vResp, err := viewerQuery(context.Background(), c)
	require.NoError(t, err)

	vPayload := (*vResp.Viewer).(*viewerQueryViewer)
	node := vPayload.User.Feed.Edges[0].Node
	assert.NotNil(t, node)
	feedEvent := (*node).(*viewerQueryViewerUserGalleryUserFeedFeedConnectionEdgesFeedEdgeNodeFeedEvent)
	assert.Equal(t, "newCaption", *feedEvent.Caption)
	edata := *(*feedEvent.EventData).(*viewerQueryViewerUserGalleryUserFeedFeedConnectionEdgesFeedEdgeNodeFeedEventEventDataGalleryUpdatedFeedEventData)
	assert.EqualValues(t, persist.ActionGalleryUpdated, *edata.Action)
	for _, c := range edata.SubEventDatas {
		ac := c.GetAction()
		if persist.Action(*ac) == persist.ActionCollectionCreated {
			ca := c.(*viewerQueryViewerUserGalleryUserFeedFeedConnectionEdgesFeedEdgeNodeFeedEventEventDataGalleryUpdatedFeedEventDataSubEventDatasCollectionCreatedFeedEventData)
			assert.Greater(t, len(ca.NewTokens), 0)
		}
		if persist.Action(*ac) == persist.ActionTokensAddedToCollection {
			ca := c.(*viewerQueryViewerUserGalleryUserFeedFeedConnectionEdgesFeedEdgeNodeFeedEventEventDataGalleryUpdatedFeedEventDataSubEventDatasTokensAddedToCollectionFeedEventData)
			assert.Greater(t, len(ca.NewTokens), 0)
		}
	}
}

func testUpdateGalleryWithNoNameChange(t *testing.T) {
	userF := newUserWithTokensFixture(t)
	c := authedHandlerClient(t, userF.id)

	response, err := updateGalleryMutation(context.Background(), c, UpdateGalleryInput{
		GalleryId: userF.galleryID,
		Name:      util.StringToPointer("newName"),
	})

	require.NoError(t, err)
	payload, ok := (*response.UpdateGallery).(*updateGalleryMutationUpdateGalleryUpdateGalleryPayload)
	if !ok {
		err := (*response.UpdateGallery).(*updateGalleryMutationUpdateGalleryErrInvalidInput)
		t.Fatal(err)
	}
	assert.NotEmpty(t, payload.Gallery.Name)

	response, err = updateGalleryMutation(context.Background(), c, UpdateGalleryInput{
		GalleryId: userF.galleryID,
	})

	require.NoError(t, err)
	payload, ok = (*response.UpdateGallery).(*updateGalleryMutationUpdateGalleryUpdateGalleryPayload)
	if !ok {
		err := (*response.UpdateGallery).(*updateGalleryMutationUpdateGalleryErrInvalidInput)
		t.Fatal(err)
	}
	assert.NotEmpty(t, payload.Gallery.Name)
}

func testUpdateGalleryWithNewCollection(t *testing.T) {
	userF := newUserWithTokensFixture(t)
	c := authedHandlerClient(t, userF.id)

	response, err := updateGalleryMutation(context.Background(), c, UpdateGalleryInput{
		GalleryId: userF.galleryID,

		CreatedCollections: []*CreateCollectionInGalleryInput{
			{
				Name:           "yay",
				CollectorsNote: "this is a note",
				Tokens:         userF.tokenIDs[:1],
				Hidden:         false,
				Layout: CollectionLayoutInput{
					Sections: []int{0},
					SectionLayout: []CollectionSectionLayoutInput{
						{
							Columns:    1,
							Whitespace: []int{},
						},
					},
				},
				TokenSettings: []CollectionTokenSettingsInput{},
				GivenID:       "wow",
			},
		},
		Order: []persist.DBID{"wow"},
	})

	require.NoError(t, err)
	payload, ok := (*response.UpdateGallery).(*updateGalleryMutationUpdateGalleryUpdateGalleryPayload)
	if !ok {
		err := (*response.UpdateGallery).(*updateGalleryMutationUpdateGalleryErrInvalidInput)
		t.Fatal(err)
	}
	assert.Len(t, payload.Gallery.Collections, 1)
	assert.Len(t, payload.Gallery.Collections[0].Tokens, 1)
}

func testViewsAreRolledUp(t *testing.T) {
	serverF := newServerFixture(t)
	userF := newUserFixture(t)
	viewerA := newUserFixture(t)
	viewerB := newUserFixture(t)
	// viewerA views gallery
	clientA := authedServerClient(t, serverF.server.URL, viewerA.id)
	viewGallery(t, clientA, userF.galleryID)
	// // viewerB views gallery
	clientB := authedServerClient(t, serverF.server.URL, viewerB.id)
	viewGallery(t, clientB, userF.galleryID)

	// TODO: Actually verify that the views get rolled up
}

func testTrendingUsers(t *testing.T) {
	serverF := newServerFixture(t)
	userA := newUserFixture(t)
	userB := newUserFixture(t)
	userC := newUserFixture(t)
	ctx := context.Background()
	c := defaultServerClient(t, serverF.server.URL)
	// view userA a few times
	for i := 0; i < 5; i++ {
		viewGallery(t, c, userA.galleryID)
	}
	// view userB a few times
	for i := 0; i < 3; i++ {
		viewGallery(t, c, userB.galleryID)
	}
	// view userC a few times
	for i := 0; i < 1; i++ {
		viewGallery(t, c, userC.galleryID)
	}
	expected := []persist.DBID{userA.id, userB.id, userC.id}
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

func newNonce(t *testing.T, c *handlerClient, w wallet) string {
	t.Helper()
	response, err := getAuthNonceMutation(context.Background(), c, chainAddressInput(w.address))
	require.NoError(t, err)
	payload := (*response.GetAuthNonce).(*getAuthNonceMutationGetAuthNonce)
	return *payload.Nonce
}

// newUser makes a GraphQL request to generate a new user
func newUser(t *testing.T, c *handlerClient, w wallet) (userID persist.DBID, username string, galleryID persist.DBID) {
	t.Helper()
	nonce := newNonce(t, c, w)
	username = "user" + persist.GenerateID().String()

	response, err := createUserMutation(context.Background(), c, authMechanismInput(w, nonce),
		CreateUserInput{Username: username},
	)

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
	c := customHandlerClient(t, handler, withJWTOpt(t, userID))

	response, err := syncTokensMutation(context.Background(), c, []Chain{"Ethereum"})

	require.NoError(t, err)
	payload := (*response.SyncTokens).(*syncTokensMutationSyncTokensSyncTokensPayload)
	tokens := make([]persist.DBID, len(payload.Viewer.User.Tokens))
	for i, token := range payload.Viewer.User.Tokens {
		tokens[i] = token.Dbid
	}
	return tokens
}

// viewGallery makes a GraphQL request to view a gallery
func viewGallery(t *testing.T, c graphql.Client, galleryID persist.DBID) {
	resp, err := viewGalleryMutation(context.Background(), c, galleryID)
	_ = (*resp.ViewGallery).(*viewGalleryMutationViewGalleryViewGalleryPayload)
	require.NoError(t, err)
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
	return &serverClient{url: host + "/glry/graphql/query"}
}

// authedServerClient provides an authenticated client to a live server
func authedServerClient(t *testing.T, host string, userID persist.DBID) *serverClient {
	return &serverClient{url: host + "/glry/graphql/query", opts: []func(*http.Request){withJWTOpt(t, userID)}}
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
