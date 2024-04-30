//go:generate go get github.com/Khan/genqlient/generate
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
	"time"

	genql "github.com/Khan/genqlient/graphql"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/limiters"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/common"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/tokenmanage"
	"github.com/mikeydub/go-gallery/tokenprocessing"
	"github.com/mikeydub/go-gallery/util"
)

type testCase struct {
	title    string
	run      func(t *testing.T)
	fixtures []fixture
}

func TestMain(t *testing.T) {
	tests := []testCase{
		{
			title:    "test graphql",
			run:      testGraphQL,
			fixtures: []fixture{useDefaultEnv, usePostgres, useRedis, useCloudTasksDirectDispatch, useAutosocial, useNotificationTopics},
		},
		{
			title:    "test syncing tokens",
			run:      testTokenSyncs,
			fixtures: []fixture{useDefaultEnv, usePostgres, useRedis, useCloudTasksDirectDispatch, useAutosocial, useTokenProcessing},
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
		{title: "should create a collection", run: testCreateCollection},
		{title: "views from multiple users are rolled up", run: testViewsAreRolledUp},
		{title: "update gallery and create a feed event", run: testUpdateGalleryWithPublish},
		{title: "update gallery and ensure name still gets set when not sent in update", run: testUpdateGalleryWithNoNameChange},
		{title: "update gallery with a new collection", run: testUpdateGalleryWithNewCollection},
		{title: "should get trending users", run: testTrendingUsers, fixtures: []fixture{usePostgres, useRedis}},
		{title: "should get trending feed events", run: testTrendingFeedEvents},
		{title: "should delete a post", run: testDeletePost},
		{title: "should get community with posts", run: testGetCommunity},
		{title: "should delete collection in gallery update", run: testUpdateGalleryDeleteCollection},
		{title: "should update user experiences", run: testUpdateUserExperiences},
		{title: "should create gallery", run: testCreateGallery},
		{title: "should move collection to new gallery", run: testMoveCollection},
		{title: "should connect social account", run: testConnectSocialAccount},
		{title: "should view a token", run: testViewToken},
		{title: "should admire a token", run: testAdmireToken},
		{title: "should send notifications", run: testSendNotifications, fixtures: []fixture{usePostgres, useRedis}},
	}
	for _, test := range tests {
		t.Run(test.title, testWithFixtures(test.run, test.fixtures...))
	}
}

func testTokenSyncs(t *testing.T) {
	tests := []testCase{
		{title: "should sync new tokens", run: testSyncNewTokens},
		{title: "should sync new tokens incrementally", run: testSyncNewTokensIncrementally},
		{title: "should sync new tokens multichain", run: testSyncNewTokensMultichain},
		{title: "should submit new tokens to tokenprocessing", run: testSyncOnlySubmitsNewTokens},
		{title: "should not submit old tokens to tokenprocessing", run: testSyncSkipsSubmittingOldTokens},
		{title: "should keep old tokens", run: testSyncKeepsOldTokens},
		{title: "should merge duplicates within provider", run: testSyncShouldMergeDuplicatesInProvider},
		{title: "should process media", run: testSyncShouldProcessMedia},
	}
	for _, test := range tests {
		t.Run(test.title, testWithFixtures(test.run, test.fixtures...))
	}
}

func testCreateUser(t *testing.T) {
	nonceF := newNonceFixture(t)
	c := defaultHandlerClient(t)
	username := "user" + persist.GenerateID().String()

	response, err := createUserMutation(context.Background(), c, authMechanismInput(nonceF.Wallet, nonceF.Nonce, nonceF.Message),
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
	response, err := userByUsernameQuery(context.Background(), defaultHandlerClient(t), userF.Username)

	require.NoError(t, err)
	payload, _ := (*response.UserByUsername).(*userByUsernameQueryUserByUsernameGalleryUser)
	assert.Equal(t, userF.Username, *payload.Username)
	assert.Equal(t, userF.ID, payload.Dbid)
}

func testUserByAddress(t *testing.T) {
	userF := newUserFixture(t)
	c := authedHandlerClient(t, userF.ID)

	response, err := userByAddressQuery(context.Background(), c, chainAddressInput(userF.Wallet.Address.String()))

	require.NoError(t, err)
	payload, _ := (*response.UserByAddress).(*userByAddressQueryUserByAddressGalleryUser)
	assert.Equal(t, userF.Username, *payload.Username)
	assert.Equal(t, userF.ID, payload.Dbid)
}

func testUserByID(t *testing.T) {
	userF := newUserFixture(t)
	response, err := userByIdQuery(context.Background(), defaultHandlerClient(t), userF.ID)

	require.NoError(t, err)
	payload, _ := (*response.UserById).(*userByIdQueryUserByIdGalleryUser)
	assert.Equal(t, userF.Username, *payload.Username)
	assert.Equal(t, userF.ID, payload.Dbid)
}

func testViewer(t *testing.T) {
	userF := newUserFixture(t)
	c := authedHandlerClient(t, userF.ID)

	response, err := viewerQuery(context.Background(), c)
	require.NoError(t, err)

	payload, _ := (*response.Viewer).(*viewerQueryViewer)
	assert.Equal(t, userF.Username, *payload.User.Username)
}

func testAddWallet(t *testing.T) {
	userF := newUserFixture(t)
	walletToAdd := newWallet(t)
	ctx := context.Background()
	c := authedHandlerClient(t, userF.ID)
	nonce, message := newNonce(t, ctx, c)

	response, err := addUserWalletMutation(ctx, c, chainAddressInput(walletToAdd.Address.String()), authMechanismInput(walletToAdd, nonce, message))

	require.NoError(t, err)
	payload, _ := (*response.AddUserWallet).(*addUserWalletMutationAddUserWalletAddUserWalletPayload)
	wallets := payload.Viewer.User.Wallets
	assert.Equal(t, walletToAdd.Address.String(), *wallets[len(wallets)-1].ChainAddress.Address)
	assert.Equal(t, Chain("Ethereum"), *wallets[len(wallets)-1].ChainAddress.Chain)
	assert.Len(t, wallets, 2)
}

func testRemoveWallet(t *testing.T) {
	userF := newUserFixture(t)
	walletToRemove := newWallet(t)
	ctx := context.Background()
	c := authedHandlerClient(t, userF.ID)
	nonce, message := newNonce(t, ctx, c)
	addResponse, err := addUserWalletMutation(ctx, c, chainAddressInput(walletToRemove.Address.String()), authMechanismInput(walletToRemove, nonce, message))
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
	nonce, message := newNonce(t, ctx, c)

	response, err := loginMutation(ctx, c, authMechanismInput(userF.Wallet, nonce, message))

	require.NoError(t, err)
	payload, _ := (*response.Login).(*loginMutationLoginLoginPayload)
	assert.NotEmpty(t, readCookie(t, c.response, auth.AuthCookieKey))
	assert.NotEmpty(t, readCookie(t, c.response, auth.RefreshCookieKey))
	assert.Equal(t, userF.Username, *payload.Viewer.User.Username)
	assert.Equal(t, userF.ID, payload.Viewer.User.Dbid)
}

func testLogout(t *testing.T) {
	userF := newUserFixture(t)
	c := authedHandlerClient(t, userF.ID)

	response, err := logoutMutation(context.Background(), c)

	require.NoError(t, err)
	assert.Empty(t, readCookie(t, c.response, auth.AuthCookieKey))
	assert.Empty(t, readCookie(t, c.response, auth.RefreshCookieKey))
	assert.Nil(t, response.Logout.Viewer)
}

func testCreateCollection(t *testing.T) {
	userF := newUserWithTokensFixture(t)
	c := authedHandlerClient(t, userF.ID)

	response, err := createCollectionMutation(context.Background(), c, CreateCollectionInput{
		GalleryId:      userF.GalleryID,
		Name:           "newCollection",
		CollectorsNote: "this is a note",
		Tokens:         userF.TokenIDs,
		Layout:         defaultLayout(),
		TokenSettings:  defaultTokenSettings(userF.TokenIDs),
		Caption:        nil,
	})

	require.NoError(t, err)
	payload := (*response.CreateCollection).(*createCollectionMutationCreateCollectionCreateCollectionPayload)
	assert.NotEmpty(t, payload.Collection.Dbid)
	assert.Len(t, payload.Collection.Tokens, len(userF.TokenIDs))
}

func testUpdateGalleryWithPublish(t *testing.T) {
	serverF := newServerFixture(t)
	userF := newUserWithTokensFixture(t)
	c := authedServerClient(t, serverF.URL, userF.ID)

	colResp, err := createCollectionMutation(context.Background(), c, CreateCollectionInput{
		GalleryId:      userF.GalleryID,
		Name:           "newCollection",
		CollectorsNote: "this is a note",
		Tokens:         userF.TokenIDs[:1],
		Layout:         defaultLayout(),
		TokenSettings:  defaultTokenSettings(userF.TokenIDs[:1]),
		Caption:        nil,
	})

	require.NoError(t, err)
	colPay := (*colResp.CreateCollection).(*createCollectionMutationCreateCollectionCreateCollectionPayload)
	assert.NotEmpty(t, colPay.Collection.Dbid)
	assert.Len(t, colPay.Collection.Tokens, 1)

	updateReponse, err := updateGalleryMutation(context.Background(), c, UpdateGalleryInput{
		GalleryId: userF.GalleryID,
		Name:      util.ToPointer("newName"),
		UpdatedCollections: []*UpdateCollectionInput{
			{
				Dbid:           colPay.Collection.Dbid,
				Tokens:         userF.TokenIDs[:2],
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
				TokenSettings: defaultTokenSettings(userF.TokenIDs[:2]),
			},
		},
		CreatedCollections: []*CreateCollectionInGalleryInput{
			{
				GivenID:        "wow",
				Tokens:         userF.TokenIDs[:3],
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
				TokenSettings: defaultTokenSettings(userF.TokenIDs[:3]),
			},
		},
		Order:  []persist.DBID{colPay.Collection.Dbid, "wow"},
		EditId: util.ToPointer("edit_id"),
	})

	require.NoError(t, err)
	require.NotNil(t, updateReponse.UpdateGallery)
	updatePayload, ok := (*updateReponse.UpdateGallery).(*updateGalleryMutationUpdateGalleryUpdateGalleryPayload)
	if !ok {
		err := (*updateReponse.UpdateGallery).(*updateGalleryMutationUpdateGalleryErrInvalidInput)
		t.Fatal(err)
	}
	assert.NotEmpty(t, updatePayload.Gallery.Name)

	update2Reponse, err := updateGalleryMutation(context.Background(), c, UpdateGalleryInput{
		GalleryId:   userF.GalleryID,
		Description: util.ToPointer("newDesc"),
		EditId:      util.ToPointer("edit_id"),
	})

	require.NoError(t, err)
	require.NotNil(t, update2Reponse.UpdateGallery)

	// Wait for event handlers to store update events
	time.Sleep(time.Second)

	// publish
	publishResponse, err := publishGalleryMutation(context.Background(), c, PublishGalleryInput{
		GalleryId: userF.GalleryID,
		EditId:    "edit_id",
		Caption:   util.ToPointer("newCaption"),
	})
	require.NoError(t, err)
	require.NotNil(t, publishResponse.PublishGallery)

	vResp, err := viewerQuery(context.Background(), c)
	require.NoError(t, err)

	vPayload := (*vResp.Viewer).(*viewerQueryViewer)
	assert.NotNil(t, vPayload.User)
	assert.NotNil(t, vPayload.User.Feed)
	assert.NotNil(t, vPayload.User.Feed.Edges)
	// Assert that feed events aren't shown on the user's activity feed
	assert.Len(t, vPayload.User.Feed.Edges, 0)
}

func testCreateGallery(t *testing.T) {
	userF := newUserWithTokensFixture(t)
	c := authedHandlerClient(t, userF.ID)

	response, err := createGalleryMutation(context.Background(), c, CreateGalleryInput{
		Name:        util.ToPointer("newGallery"),
		Description: util.ToPointer("this is a description"),
		Position:    "a1",
	})

	require.NoError(t, err)
	payload := (*response.CreateGallery).(*createGalleryMutationCreateGalleryCreateGalleryPayload)
	assert.NotEmpty(t, payload.Gallery.Dbid)
	assert.Equal(t, "newGallery", *payload.Gallery.Name)
	assert.Equal(t, "this is a description", *payload.Gallery.Description)
	assert.Equal(t, "a1", *payload.Gallery.Position)
}

func testMoveCollection(t *testing.T) {
	userF := newUserWithTokensFixture(t)
	c := authedHandlerClient(t, userF.ID)

	createResp, err := createCollectionMutation(context.Background(), c, CreateCollectionInput{
		GalleryId:      userF.GalleryID,
		Name:           "newCollection",
		CollectorsNote: "this is a note",
		Tokens:         userF.TokenIDs,
		Layout:         defaultLayout(),
		TokenSettings:  defaultTokenSettings(userF.TokenIDs),
		Caption:        nil,
	})

	require.NoError(t, err)
	createPayload := (*createResp.CreateCollection).(*createCollectionMutationCreateCollectionCreateCollectionPayload)
	assert.NotEmpty(t, createPayload.Collection.Dbid)

	createGalResp, err := createGalleryMutation(context.Background(), c, CreateGalleryInput{
		Name:        util.ToPointer("newGallery"),
		Description: util.ToPointer("this is a description"),
		Position:    "a1",
	})

	require.NoError(t, err)
	createGalPayload := (*createGalResp.CreateGallery).(*createGalleryMutationCreateGalleryCreateGalleryPayload)
	assert.NotEmpty(t, createGalPayload.Gallery.Dbid)

	response, err := moveCollectionToGallery(context.Background(), c, MoveCollectionToGalleryInput{
		SourceCollectionId: createPayload.Collection.Dbid,
		TargetGalleryId:    createGalPayload.Gallery.Dbid,
	})

	require.NoError(t, err)
	payload := (*response.MoveCollectionToGallery).(*moveCollectionToGalleryMoveCollectionToGalleryMoveCollectionToGalleryPayload)
	assert.NotEmpty(t, payload.OldGallery.Dbid)
	assert.Len(t, payload.OldGallery.Collections, 0)
	assert.NotEmpty(t, payload.NewGallery.Dbid)
	assert.Len(t, payload.NewGallery.Collections, 1)

}

func testUpdateUserExperiences(t *testing.T) {
	userF := newUserFixture(t)
	c := authedHandlerClient(t, userF.ID)

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

func testConnectSocialAccount(t *testing.T) {
	userF := newUserFixture(t)
	c := authedHandlerClient(t, userF.ID)

	connectResp, err := connectSocialAccount(context.Background(), c, SocialAuthMechanism{
		Debug: &DebugSocialAuth{
			Provider: SocialAccountTypeTwitter,
			Id:       "123",
			Username: "test",
		},
	}, true)
	require.NoError(t, err)

	payload := (*connectResp.ConnectSocialAccount).(*connectSocialAccountConnectSocialAccountConnectSocialAccountPayload)
	assert.Equal(t, payload.Viewer.SocialAccounts.Twitter.Username, "test")
	assert.True(t, payload.Viewer.SocialAccounts.Twitter.Display)

	viewerResp, err := viewerQuery(context.Background(), c)
	require.NoError(t, err)
	viewerPayload := (*viewerResp.Viewer).(*viewerQueryViewer)
	assert.Equal(t, viewerPayload.User.SocialAccounts.Twitter.Username, "test")

	updateDisplayedResp, err := updateSocialAccountDisplayed(context.Background(), c, UpdateSocialAccountDisplayedInput{
		Type:      SocialAccountTypeTwitter,
		Displayed: false,
	})

	require.NoError(t, err)

	updateDisplayedPayload := (*updateDisplayedResp.UpdateSocialAccountDisplayed).(*updateSocialAccountDisplayedUpdateSocialAccountDisplayedUpdateSocialAccountDisplayedPayload)
	assert.Equal(t, updateDisplayedPayload.Viewer.SocialAccounts.Twitter.Username, "test")
	assert.False(t, updateDisplayedPayload.Viewer.SocialAccounts.Twitter.Display)

	// For now, we're always returning the user's social accounts despite preference setting.
	// If there's community significant pushback about this then we can reinstate the feature.
	// dc := defaultHandlerClient(t)
	// userResp, err := userByIdQuery(context.Background(), dc, userF.ID)
	// require.NoError(t, err)
	// userPayload := (*userResp.UserById).(*userByIdQueryUserByIdGalleryUser)
	// assert.Nil(t, userPayload.SocialAccounts.Twitter)

	disconnectResp, err := disconnectSocialAccount(context.Background(), c, SocialAccountTypeTwitter)
	require.NoError(t, err)

	disconnectPayload := (*disconnectResp.DisconnectSocialAccount).(*disconnectSocialAccountDisconnectSocialAccountDisconnectSocialAccountPayload)
	assert.Nil(t, disconnectPayload.Viewer.SocialAccounts.Twitter)

}

func testUpdateGalleryDeleteCollection(t *testing.T) {
	userF := newUserWithTokensFixture(t)
	c := authedHandlerClient(t, userF.ID)

	colResp, err := createCollectionMutation(context.Background(), c, CreateCollectionInput{
		GalleryId:      userF.GalleryID,
		Name:           "newCollection",
		CollectorsNote: "this is a note",
		Tokens:         userF.TokenIDs[:1],
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
				TokenId:    userF.TokenIDs[0],
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
		GalleryId:          userF.GalleryID,
		DeletedCollections: []persist.DBID{colPay.Collection.Dbid},
		Order:              []persist.DBID{},
	})

	require.NoError(t, err)
	require.NotNil(t, response.UpdateGallery)
	payload, ok := (*response.UpdateGallery).(*updateGalleryMutationUpdateGalleryUpdateGalleryPayload)
	if !ok {
		err := (*response.UpdateGallery).(*updateGalleryMutationUpdateGalleryErrInvalidInput)
		t.Fatal(err)
	}
	assert.Len(t, payload.Gallery.Collections, 0)
}

func testUpdateGalleryWithNoNameChange(t *testing.T) {
	userF := newUserWithTokensFixture(t)
	c := authedHandlerClient(t, userF.ID)

	response, err := updateGalleryMutation(context.Background(), c, UpdateGalleryInput{
		GalleryId: userF.GalleryID,
		Name:      util.ToPointer("newName"),
	})

	require.NoError(t, err)
	payload, ok := (*response.UpdateGallery).(*updateGalleryMutationUpdateGalleryUpdateGalleryPayload)
	if !ok {
		err := (*response.UpdateGallery).(*updateGalleryMutationUpdateGalleryErrInvalidInput)
		t.Fatal(err)
	}
	assert.NotEmpty(t, payload.Gallery.Name)

	response, err = updateGalleryMutation(context.Background(), c, UpdateGalleryInput{
		GalleryId: userF.GalleryID,
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
	c := authedHandlerClient(t, userF.ID)

	response, err := updateGalleryMutation(context.Background(), c, UpdateGalleryInput{
		GalleryId: userF.GalleryID,

		CreatedCollections: []*CreateCollectionInGalleryInput{
			{
				Name:           "yay",
				CollectorsNote: "this is a note",
				Tokens:         userF.TokenIDs[:1],
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
	bob := newUserFixture(t)
	alice := newUserFixture(t)
	ctx := context.Background()
	// bob views gallery
	client := authedServerClient(t, serverF.URL, bob.ID)
	viewGallery(t, ctx, client, userF.GalleryID)
	// // alice views gallery
	client = authedServerClient(t, serverF.URL, alice.ID)
	viewGallery(t, ctx, client, userF.GalleryID)

	// TODO: Actually verify that the views get rolled up
}

func testTrendingUsers(t *testing.T) {
	t.Skip("This test is pretty flaky, skipping for now")
	serverF := newServerFixture(t)
	bob := newUserWithFeedEntitiesFixture(t)
	alice := newUserWithFeedEntitiesFixture(t)
	dave := newUserWithFeedEntitiesFixture(t)
	ctx := context.Background()
	var c *serverClient
	// view bob a few times
	for i := 0; i < 5; i++ {
		viewer := newUserFixture(t)
		c = authedServerClient(t, serverF.URL, viewer.ID)
		viewGallery(t, ctx, c, bob.GalleryID)
	}
	// view alice a few times
	for i := 0; i < 3; i++ {
		viewer := newUserFixture(t)
		c = authedServerClient(t, serverF.URL, viewer.ID)
		viewGallery(t, ctx, c, alice.GalleryID)
	}
	// view dave a few times
	for i := 0; i < 1; i++ {
		viewer := newUserFixture(t)
		c = authedServerClient(t, serverF.URL, viewer.ID)
		viewGallery(t, ctx, c, dave.GalleryID)
	}
	expected := []persist.DBID{bob.ID, alice.ID, dave.ID}
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

	// Wait for event handlers to store views
	time.Sleep(time.Second)

	t.Run("should pull the last 5 days", func(t *testing.T) {
		actual := getTrending(t, "LAST_5_DAYS")
		assert.EqualValues(t, expected, actual)
	})

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
	userF := newUserWithFeedEntitiesFixture(t)
	c := authedHandlerClient(t, userF.ID)
	// four total interactions
	admirePost(t, ctx, c, userF.PostIDs[0])
	commentOnPost(t, ctx, c, userF.PostIDs[0], "a")
	commentOnPost(t, ctx, c, userF.PostIDs[0], "b")
	commentOnPost(t, ctx, c, userF.PostIDs[0], "c")
	// three total interactions
	admirePost(t, ctx, c, userF.PostIDs[1])
	commentOnPost(t, ctx, c, userF.PostIDs[1], "a")
	commentOnPost(t, ctx, c, userF.PostIDs[1], "b")
	// one total interactions
	admirePost(t, ctx, c, userF.PostIDs[2])

	// one total interactions
	admireFeedEvent(t, ctx, c, userF.FeedEventIDs[2])
	expected := []persist.DBID{
		userF.PostIDs[2],
		userF.PostIDs[1],
		userF.PostIDs[0],
	}

	actual := trendingFeedEvents(t, ctx, c, 4, true)

	assert.Equal(t, expected, actual)
}

func testDeletePost(t *testing.T) {
	ctx := context.Background()
	userF := newUserWithFeedEntitiesFixture(t)
	c := authedHandlerClient(t, userF.ID)
	deletePost(t, ctx, c, userF.PostIDs[0])
	actual := globalFeedEvents(t, ctx, c, 4, true)
	assert.False(t, util.Contains(actual, userF.PostIDs[0]))
}

func testGetCommunity(t *testing.T) {
	ctx := context.Background()
	userF := newUserWithFeedEntitiesFixture(t)
	c := authedHandlerClient(t, userF.ID)
	contract := contractAddressByTokenID(t, ctx, c, userF.TokenIDs[0])
	communityByAddress(t, ctx, c, ChainAddressInput{
		Address: contract,
		Chain:   ChainEthereum,
	})
}

func testAdmireToken(t *testing.T) {
	ctx := context.Background()
	userF := newUserFixture(t)
	c := authedHandlerClient(t, userF.ID)
	alice := newUserWithTokensFixture(t)
	aliceTokenAdmireResp := admireToken(t, ctx, c, alice.TokenIDs[0])
	assert.Equal(t, aliceTokenAdmireResp, alice.TokenIDs[0])
}

func testViewToken(t *testing.T) {
	ctx := context.Background()
	userF := newUserWithTokensFixture(t)
	alice := newUserFixture(t)
	bob := newUserFixture(t)
	c := authedHandlerClient(t, userF.ID)
	c2 := authedHandlerClient(t, alice.ID)
	c3 := authedHandlerClient(t, bob.ID)

	colResp, err := createCollectionMutation(ctx, c, CreateCollectionInput{
		GalleryId:      userF.GalleryID,
		Name:           "newCollection",
		CollectorsNote: "this is a note",
		Tokens:         userF.TokenIDs[:1],
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
				TokenId:    userF.TokenIDs[0],
				RenderLive: false,
			},
		},
		Caption: nil,
	})

	require.NoError(t, err)
	colPay := (*colResp.CreateCollection).(*createCollectionMutationCreateCollectionCreateCollectionPayload)
	assert.NotEmpty(t, colPay.Collection.Dbid)
	assert.Len(t, colPay.Collection.Tokens, 1)

	responseAliceViewToken := viewToken(t, ctx, c2, userF.TokenIDs[0], colPay.Collection.Dbid)
	responseBobViewToken := viewToken(t, ctx, c3, userF.TokenIDs[0], colPay.Collection.Dbid)
	assert.NotEmpty(t, responseAliceViewToken)
	assert.NotEmpty(t, responseBobViewToken)
}

func testSendNotifications(t *testing.T) {
	ctx := context.Background()
	pushService := newPushNotificationServiceFixture(t)
	userF := newUserWithFeedEntitiesFixture(t)
	alice := newUserWithFeedEntitiesFixture(t)
	c := authedHandlerClient(t, userF.ID)
	c2 := authedHandlerClient(t, alice.ID)
	registerPushToken(t, ctx, c)
	registerPushToken(t, ctx, c2)

	admirePost(t, ctx, c2, userF.PostIDs[0])
	commentOnPost(t, ctx, c2, userF.PostIDs[0], "post comment 1")
	commentOnPost(t, ctx, c2, userF.PostIDs[0], "post comment 2")
	admireFeedEvent(t, ctx, c2, userF.FeedEventIDs[0])
	commentID := commentOnFeedEvent(t, ctx, c2, userF.FeedEventIDs[0], "feed event comment")
	admireComment(t, ctx, c, commentID)

	require.Eventuallyf(t, func() bool {
		return len(pushService.SentNotificationBodies) == 6
	}, time.Second*30, time.Second, "expected 6 push notifications to be sent, got %d", len(pushService.SentNotificationBodies))

	assert.Empty(t, pushService.Errors)
	assert.Contains(t, pushService.SentNotificationBodies, fmt.Sprintf("%s admired your post", alice.Username))
	assert.Contains(t, pushService.SentNotificationBodies, fmt.Sprintf("%s commented on your post: post comment 1", alice.Username))
	assert.Contains(t, pushService.SentNotificationBodies, fmt.Sprintf("%s commented on your post: post comment 2", alice.Username))
	assert.Contains(t, pushService.SentNotificationBodies, fmt.Sprintf("%s admired your gallery update", alice.Username))
	assert.Contains(t, pushService.SentNotificationBodies, fmt.Sprintf("%s commented on your gallery update: feed event comment", alice.Username))
	assert.Contains(t, pushService.SentNotificationBodies, fmt.Sprintf("%s admired your comment", userF.Username))

	// Check viewer notifications
	response, err := notificationsForViewerQuery(ctx, c)
	require.NoError(t, err)
	payload := (*response.GetViewer()).(*notificationsForViewerQueryViewer)
	require.NotNil(t, payload)
	assert.Equal(t, 5, *(payload.GetNotifications().GetUnseenCount()))
	// Check other user's notifications
	response, err = notificationsForViewerQuery(ctx, c2)
	require.NoError(t, err)
	payload = (*response.GetViewer()).(*notificationsForViewerQueryViewer)
	require.NotNil(t, payload)
	assert.Equal(t, 1, *(payload.GetNotifications().GetUnseenCount()))
}

func testSyncNewTokens(t *testing.T) {
	userF := newUserFixture(t)
	provider := defaultStubProvider(userF.Wallet.Address)
	providers := multichain.ProviderLookup{persist.ChainETH: provider}
	ctx := context.Background()

	t.Run("should sync new tokens", func(t *testing.T) {
		h := handlerWithProviders(t, &noopSubmitter{}, providers)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		require.NoError(t, err)
		payload := (*response.SyncTokens).(*syncTokensMutationSyncTokensSyncTokensPayload)
		assert.Len(t, payload.Viewer.User.Tokens, len(provider.Tokens))
	})

	t.Run("should not duplicate tokens from repeat syncs", func(t *testing.T) {
		h := handlerWithProviders(t, &noopSubmitter{}, providers)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		require.NoError(t, err)
		payload := (*response.SyncTokens).(*syncTokensMutationSyncTokensSyncTokensPayload)
		assert.Len(t, payload.Viewer.User.Tokens, len(provider.Tokens))
	})

	t.Run("should not delete tokens if provider fails", func(t *testing.T) {
		userF := newUserWithTokensFixture(t)
		require.Greater(t, len(userF.TokenIDs), 0)
		providers := multichain.ProviderLookup{persist.ChainETH: newStubProvider(withReturnError(fmt.Errorf("can't get tokens")))}
		h := handlerWithProviders(t, &noopSubmitter{}, providers)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		// Sync tokens with a failing provider
		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		// Expect the sync to fail because of bad provider
		require.NoError(t, err)
		_ = (*response.SyncTokens).(*syncTokensMutationSyncTokensErrSyncFailed)
		// Admire all the existing tokens to confirm that they are still displayable
		for _, id := range userF.TokenIDs {
			admireToken(t, ctx, c, id)
		}
	})
}

func testSyncNewTokensIncrementally(t *testing.T) {
	userF := newUserFixture(t)
	provider := defaultStubProvider(userF.Wallet.Address)
	providers := multichain.ProviderLookup{persist.ChainETH: provider}
	ctx := context.Background()

	tr := util.ToPointer(true)
	t.Run("should sync new tokens incrementally", func(t *testing.T) {
		h := handlerWithProviders(t, &noopSubmitter{}, providers)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, tr)

		require.NoError(t, err)
		payload := (*response.SyncTokens).(*syncTokensMutationSyncTokensSyncTokensPayload)
		assert.Len(t, payload.Viewer.User.Tokens, len(provider.Tokens))
	})

	t.Run("should not duplicate tokens from repeat incremental syncs", func(t *testing.T) {
		h := handlerWithProviders(t, &noopSubmitter{}, providers)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, tr)

		require.NoError(t, err)
		payload := (*response.SyncTokens).(*syncTokensMutationSyncTokensSyncTokensPayload)
		assert.Len(t, payload.Viewer.User.Tokens, len(provider.Tokens))
	})

}

func testSyncNewTokensMultichain(t *testing.T) {
	userF := newUserFixture(t)
	provider := defaultStubProvider(userF.Wallet.Address)
	contract := common.ChainAgnosticContract{Address: "0x124", Descriptors: common.ChainAgnosticContractDescriptors{Name: "wow"}}
	secondProvider := newStubProvider(withDummyTokenN(contract, userF.Wallet.Address, 10))
	providers := multichain.ProviderLookup{persist.ChainETH: provider, persist.ChainOptimism: secondProvider}
	h := handlerWithProviders(t, &noopSubmitter{}, providers)
	c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))
	ctx := context.Background()

	t.Run("should sync tokens from multiple chains", func(t *testing.T) {
		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum, ChainOptimism}, nil)
		require.NoError(t, err)
		payload := (*response.SyncTokens).(*syncTokensMutationSyncTokensSyncTokensPayload)
		assert.Len(t, payload.Viewer.User.Tokens, len(provider.Tokens)*2)
	})
}

func testSyncOnlySubmitsNewTokens(t *testing.T) {
	userF := newUserFixture(t)
	provider := newStubProvider(withDummyTokenN(common.ChainAgnosticContract{Address: "0xdead"}, userF.Wallet.Address, 10))
	providers := multichain.ProviderLookup{persist.ChainETH: provider}
	submitter := &recorderSubmitter{}
	h := handlerWithProviders(t, submitter, providers)
	c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))
	// Ideally this compares against expected values, but mocks seems to behave weirdly with slices
	submitter.On("SubmitNewTokens", mock.Anything, mock.Anything).Times(1).Return(nil)

	_, err := syncTokensMutation(context.Background(), c, []Chain{ChainEthereum}, nil)

	require.NoError(t, err)
	submitter.AssertExpectations(t)
	tokens := submitter.Calls[0].Arguments.Get(1).([]persist.DBID)
	assert.Len(t, tokens, len(provider.Tokens))
}

func testSyncSkipsSubmittingOldTokens(t *testing.T) {
	userF := newUserFixture(t)
	ctx := context.Background()
	dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, "0xffff")
	provider := newStubProvider(dummyTokenOpt)
	providers := multichain.ProviderLookup{persist.ChainETH: provider}
	h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/image", dummyTokenOpt)
	c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))
	// Sync tokens
	_, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)
	require.NoError(t, err)

	// Then sync again, with a provider that returns the same tokens
	submitter := &recorderSubmitter{}
	h = handlerWithProviders(t, submitter, providers)
	c = customHandlerClient(t, h, withJWTOpt(t, userF.ID))

	_, err = syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)
	require.NoError(t, err)
	submitter.AssertNotCalled(t, "SubmitNewTokens")
}

func testSyncKeepsOldTokens(t *testing.T) {
	userF := newUserWithTokensFixture(t)
	initialTokensLen := len(userF.TokenIDs)
	newTokensLen := 4
	provider := newStubProvider(withDummyTokenN(common.ChainAgnosticContract{Address: "0x1337"}, userF.Wallet.Address, newTokensLen))
	providers := multichain.ProviderLookup{persist.ChainETH: provider}
	h := handlerWithProviders(t, &noopSubmitter{}, providers)
	c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

	response, err := syncTokensMutation(context.Background(), c, []Chain{ChainEthereum}, nil)

	require.NoError(t, err)
	assertSyncedTokens(t, response, err, initialTokensLen+newTokensLen)
}

func testSyncShouldMergeDuplicatesInProvider(t *testing.T) {
	userF := newUserFixture(t)
	token := dummyToken(userF.Wallet.Address)
	contract := common.ChainAgnosticContract{Address: token.ContractAddress, Descriptors: common.ChainAgnosticContractDescriptors{
		Name: "someContract",
	}}
	provider := newStubProvider(
		withContracts([]common.ChainAgnosticContract{contract}),
		withTokens([]common.ChainAgnosticToken{token, token}),
	)
	providers := multichain.ProviderLookup{persist.ChainETH: provider}
	h := handlerWithProviders(t, &noopSubmitter{}, providers)
	c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

	response, err := syncTokensMutation(context.Background(), c, []Chain{ChainEthereum}, nil)

	assertSyncedTokens(t, response, err, 1)
}

// newDummyMetadataProviderFixture creates a new handler configured with a stubbed provider that reads from a fixed dummy metadata server endpoint
func newDummyMetadataProviderFixture(t *testing.T, ctx context.Context, chain persist.Chain, ownerAddress persist.Address, dummyEndpointToHit string, opts ...providerOpt) http.Handler {
	metadataServerF := newMetadataServerFixture(t)
	metadataFetchF := fetchFromDummyEndpoint(metadataServerF.URL, dummyEndpointToHit)
	pOpts := append([]providerOpt{withFetchMetadata(metadataFetchF)}, opts...)
	provider := newStubProvider(pOpts...)
	providers := multichain.ProviderLookup{chain: provider}
	c := server.ClientInit(ctx)
	mc := newMultichainProvider(c, &noopSubmitter{}, providers)
	t.Cleanup(func() { c.Close() })
	submitter := &httpSubmitter{
		Handler:  tokenprocessing.CoreInitServer(ctx, c, &mc),
		Method:   http.MethodPost,
		Endpoint: "/media/process",
	}
	return handlerWithProviders(t, submitter, providers)
}

func testSyncShouldProcessMedia(t *testing.T) {

	t.Run("should process image", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0x0"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/image", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaImageMedia](*tokens[0].Media)
		assert.Equal(t, string(persist.MediaTypeImage), *media.MediaType)
		assert.NotEmpty(t, *media.MediaURL)
	})

	t.Run("should process video", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0x1423897231"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/video", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaVideoMedia](*tokens[0].Media)
		assert.Equal(t, string(persist.MediaTypeVideo), *media.MediaType)
		assert.NotEmpty(t, *media.MediaURL)
	})

	t.Run("should process iframe", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0x232474823"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/iframe", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaHtmlMedia](*tokens[0].Media)
		assert.Equal(t, string(persist.MediaTypeHTML), *media.MediaType)
		assert.NotEmpty(t, *media.MediaURL)
	})

	t.Run("should process gif", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0x342789"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/gif", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaVideoMedia](*tokens[0].Media)
		assert.Equal(t, string(persist.MediaTypeGIF), *media.MediaType)
		assert.NotEmpty(t, *media.MediaURL)
	})

	t.Run("should process bad metadata", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0x4234789123"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/bad", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaSyncingMedia](*tokens[0].Media)
		assert.Equal(t, string(persist.MediaTypeSyncing), *media.MediaType)
	})

	t.Run("should process missing metadata", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0x523489"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/notfound", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaSyncingMedia](*tokens[0].Media)
		assert.Equal(t, string(persist.MediaTypeSyncing), *media.MediaType)
	})

	t.Run("should process bad media", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0x612387192"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/media/bad", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaSyncingMedia](*tokens[0].Media)
		assert.Equal(t, string(persist.MediaTypeSyncing), *media.MediaType)
	})

	t.Run("should process missing media", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0x72342897"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/media/notfound", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaSyncingMedia](*tokens[0].Media)
		assert.Equal(t, string(persist.MediaTypeSyncing), *media.MediaType)
	})

	t.Run("should process svg", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0x821347892"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/svg", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaImageMedia](*tokens[0].Media)
		assert.Equal(t, string(persist.MediaTypeSVG), *media.MediaType)
		assert.NotEmpty(t, *media.MediaURL)
	})

	t.Run("should process base64 encoded svg", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0x9123487912"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/base64svg", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaImageMedia](*tokens[0].Media)
		assert.Equal(t, string(persist.MediaTypeSVG), *media.MediaType)
		assert.NotEmpty(t, *media.MediaURL)
	})

	t.Run("should process base64 encoded metadata", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0xa123781"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/base64", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaImageMedia](*tokens[0].Media)
		assert.Equal(t, *media.MediaType, string(persist.MediaTypeImage))
		assert.NotEmpty(t, *media.MediaURL)
	})

	t.Run("should process ipfs", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0xb1234891"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/media/ipfs", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaImageMedia](*tokens[0].Media)
		assert.Equal(t, *media.MediaType, string(persist.MediaTypeImage))
		assert.NotEmpty(t, *media.MediaURL)
	})

	t.Run("should process bad dns", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0xc42358972"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/media/dnsbad", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaSyncingMedia](*tokens[0].Media)
		assert.Equal(t, string(persist.MediaTypeSyncing), *media.MediaType)
	})

	t.Run("should process different keyword", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0xd1238901"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/differentkeyword", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaImageMedia](*tokens[0].Media)
		assert.Equal(t, string(persist.MediaTypeImage), *media.MediaType)
		assert.NotEmpty(t, *media.MediaURL)
	})

	t.Run("should process wrong keyword", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0xe234902"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/wrongkeyword", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaVideoMedia](*tokens[0].Media)
		assert.Equal(t, string(persist.MediaTypeVideo), *media.MediaType)
		assert.NotEmpty(t, *media.MediaURL)
	})

	t.Run("should process animation", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0xf2348912"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/animation", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaGltfMedia](*tokens[0].Media)
		assert.Equal(t, string(persist.MediaTypeAnimation), *media.MediaType)
		assert.NotEmpty(t, *media.MediaURL)
	})

	t.Run("should process pdf", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0x101231789"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/pdf", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaPdfMedia](*tokens[0].Media)
		assert.Equal(t, string(persist.MediaTypePDF), *media.MediaType)
		assert.NotEmpty(t, *media.MediaURL)
	})

	t.Run("should process text", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0x11123891"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/text", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaTextMedia](*tokens[0].Media)
		assert.Equal(t, string(persist.MediaTypeText), *media.MediaType)
		assert.NotEmpty(t, *media.MediaURL)
	})

	t.Run("should process bad image", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		dummyTokenOpt := withDummyTokenID(userF.Wallet.Address, persist.HexTokenID("0x2348971232"))
		h := newDummyMetadataProviderFixture(t, ctx, persist.ChainETH, userF.Wallet.Address, "/metadata/badimage", dummyTokenOpt)
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum}, nil)

		tokens := assertSyncedTokens(t, response, err, 1)
		media := waitForSynced[*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaImageMedia](*tokens[0].Media)
		assert.Equal(t, string(persist.MediaTypeImage), *media.MediaType)
		assert.NotEmpty(t, *media.MediaURL)
	})
}

// Ideally testing syncs are synchronous, but for now we have to wait for the media to be processed
func waitForSynced[T any](media any) T {
	_, ok := (media).(T)
	for i := 0; i < 20 && !ok; i++ {
		<-time.After(time.Second)
		t, ok := (media).(T)
		if ok {
			return t
		}
	}
	// let it fail
	return media.(T)
}

func assertSyncedTokens(t *testing.T, response *syncTokensMutationResponse, err error, expectedLen int) []*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensToken {
	t.Helper()
	require.NoError(t, err)
	payload := (*response.SyncTokens).(*syncTokensMutationSyncTokensSyncTokensPayload)
	assert.Len(t, payload.Viewer.User.Tokens, expectedLen)
	return payload.Viewer.User.Tokens
}

// authMechanismInput signs a nonce with an ethereum wallet
func authMechanismInput(w wallet, nonce string, message string) AuthMechanism {
	return AuthMechanism{
		Eoa: &EoaAuth{
			Nonce:     nonce,
			Message:   message,
			Signature: w.Sign(message),
			ChainPubKey: ChainPubKeyInput{
				PubKey: w.Address.String(),
				Chain:  "Ethereum",
			},
		},
	}
}

func chainAddressInput(address string) ChainAddressInput {
	return ChainAddressInput{Address: address, Chain: "Ethereum"}
}

type wallet struct {
	PKey    *ecdsa.PrivateKey
	PubKey  *ecdsa.PublicKey
	Address persist.Address
}

func (w *wallet) Sign(msg string) string {
	sig, err := crypto.Sign(crypto.Keccak256([]byte(msg)), w.PKey)
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
		PKey:    pk,
		PubKey:  pubKey,
		Address: persist.Address(address),
	}
}

func newNonce(t *testing.T, ctx context.Context, c genql.Client) (string, string) {
	t.Helper()
	response, err := getAuthNonceMutation(ctx, c)
	require.NoError(t, err)
	payload := (*response.GetAuthNonce).(*getAuthNonceMutationGetAuthNonce)
	return *payload.Nonce, *payload.Message
}

// registerPushtoken makes a GraphQL request to register a push token for an authenticated user
func registerPushToken(t *testing.T, ctx context.Context, c genql.Client) {
	t.Helper()
	_, err := registerPushTokenMutation(ctx, c, persist.GenerateID().String())
	require.NoError(t, err)
}

// newUser makes a GraphQL request to generate a new user
func newUser(t *testing.T, ctx context.Context, c genql.Client, w wallet) (userID persist.DBID, username string, galleryID persist.DBID) {
	t.Helper()
	nonce, message := newNonce(t, ctx, c)
	username = "user" + persist.GenerateID().String()

	response, err := createUserMutation(ctx, c, authMechanismInput(w, nonce, message),
		CreateUserInput{Username: username},
	)

	require.NoError(t, err)
	payload := (*response.CreateUser).(*createUserMutationCreateUserCreateUserPayload)
	return payload.Viewer.User.Dbid, username, payload.Viewer.User.Galleries[0].Dbid
}

// syncTokens makes a GraphQL request to sync a user's wallet
func syncTokens(t *testing.T, ctx context.Context, c genql.Client, userID persist.DBID) []persist.DBID {
	t.Helper()
	resp, err := syncTokensMutation(ctx, c, []Chain{"Ethereum"}, nil)
	require.NoError(t, err)
	if err, ok := (*resp.SyncTokens).(*syncTokensMutationSyncTokensErrSyncFailed); ok {
		t.Fatal(err.Message)
	}
	payload := (*resp.SyncTokens).(*syncTokensMutationSyncTokensSyncTokensPayload)
	tokens := make([]persist.DBID, len(payload.Viewer.User.Tokens))
	for i, token := range payload.Viewer.User.Tokens {
		tokens[i] = token.Dbid
	}
	return tokens
}

// viewGallery makes a GraphQL request to view a gallery
func viewGallery(t *testing.T, ctx context.Context, c genql.Client, galleryID persist.DBID) {
	t.Helper()
	resp, err := viewGalleryMutation(ctx, c, galleryID)
	require.NoError(t, err)
	_ = (*resp.ViewGallery).(*viewGalleryMutationViewGalleryViewGalleryPayload)
}

// viewToken makes a GraphQL request to view a token
func viewToken(t *testing.T, ctx context.Context, c genql.Client, tokenID persist.DBID, collectionID persist.DBID) *viewTokenMutationViewTokenViewTokenPayload {
	t.Helper()
	resp, err := viewTokenMutation(ctx, c, tokenID, collectionID)
	require.NoError(t, err)
	payload := (*resp.ViewToken).(*viewTokenMutationViewTokenViewTokenPayload)
	return payload
}

// createCollection makes a GraphQL request to create a collection
func createCollection(t *testing.T, ctx context.Context, c genql.Client, input CreateCollectionInput) persist.DBID {
	t.Helper()
	resp, err := createCollectionMutation(ctx, c, input)
	require.NoError(t, err)
	payload := (*resp.CreateCollection).(*createCollectionMutationCreateCollectionCreateCollectionPayload)
	return payload.Collection.Dbid
}

func createPost(t *testing.T, ctx context.Context, c genql.Client, input PostTokensInput) persist.DBID {
	t.Helper()
	resp, err := postTokens(ctx, c, input)
	require.NoError(t, err)
	payload := (*resp.PostTokens).(*postTokensPostTokensPostTokensPayload)
	return payload.Post.Dbid
}

// globalFeedEvents makes a GraphQL request to return existing feed events
func globalFeedEvents(t *testing.T, ctx context.Context, c genql.Client, limit int, includePosts bool) []persist.DBID {
	t.Helper()
	resp, err := globalFeedQuery(ctx, c, &limit, includePosts)
	require.NoError(t, err)
	feedEntities := make([]persist.DBID, len(resp.GlobalFeed.Edges))
	for i, event := range resp.GlobalFeed.Edges {
		switch e := (*event.Node).(type) {
		case *globalFeedQueryGlobalFeedFeedConnectionEdgesFeedEdgeNodeFeedEvent:
			feedEntities[i] = e.Dbid
		case *globalFeedQueryGlobalFeedFeedConnectionEdgesFeedEdgeNodePost:
			feedEntities[i] = e.Dbid
		default:
			t.Fatalf("unexpected type %T", e)
		}

	}
	return feedEntities
}

// trendingFeedEvents makes a GraphQL request to return trending feedEvents
func trendingFeedEvents(t *testing.T, ctx context.Context, c genql.Client, limit int, includePosts bool) []persist.DBID {
	t.Helper()
	resp, err := trendingFeedQuery(ctx, c, &limit, includePosts)
	require.NoError(t, err)
	feedEvents := make([]persist.DBID, len(resp.TrendingFeed.Edges))
	for i, event := range resp.TrendingFeed.Edges {
		switch e := (*event.Node).(type) {
		case *trendingFeedQueryTrendingFeedFeedConnectionEdgesFeedEdgeNodeFeedEvent:
			feedEvents[i] = e.Dbid
		case *trendingFeedQueryTrendingFeedFeedConnectionEdgesFeedEdgeNodePost:
			feedEvents[i] = e.Dbid
		default:
			panic("unknown event type")
		}

	}
	return feedEvents
}

// admireFeedEvent makes a GraphQL request to admire a feed event
func admireFeedEvent(t *testing.T, ctx context.Context, c genql.Client, feedEventID persist.DBID) {
	t.Helper()
	resp, err := admireFeedEventMutation(ctx, c, feedEventID)
	require.NoError(t, err)
	_ = (*resp.AdmireFeedEvent).(*admireFeedEventMutationAdmireFeedEventAdmireFeedEventPayload)
}

// commentOnFeedEvent makes a GraphQL request to admire a feed event
func commentOnFeedEvent(t *testing.T, ctx context.Context, c genql.Client, feedEventID persist.DBID, comment string) persist.DBID {
	t.Helper()
	resp, err := commentOnFeedEventMutation(ctx, c, feedEventID, comment)
	require.NoError(t, err)
	return (*resp.CommentOnFeedEvent).(*commentOnFeedEventMutationCommentOnFeedEventCommentOnFeedEventPayload).Comment.Dbid
}

// admirePost makes a GraphQL request to admire a post
func admirePost(t *testing.T, ctx context.Context, c genql.Client, postID persist.DBID) {
	t.Helper()
	resp, err := admirePostMutation(ctx, c, postID)
	require.NoError(t, err)

	_ = (*resp.AdmirePost).(*admirePostMutationAdmirePostAdmirePostPayload)
}

// admireToken makes a GraphQL request to admire a token
func admireToken(t *testing.T, ctx context.Context, c genql.Client, tokenID persist.DBID) persist.DBID {
	t.Helper()
	resp, err := admireTokenMutation(ctx, c, tokenID)
	require.NoError(t, err)
	payload := (*resp.AdmireToken).(*admireTokenMutationAdmireTokenAdmireTokenPayload)
	return payload.Token.Dbid
}

// admireComment makes a GraphQL request to admire a comment
func admireComment(t *testing.T, ctx context.Context, c genql.Client, commentID persist.DBID) {
	t.Helper()
	resp, err := admireCommentMutation(ctx, c, commentID)
	require.NoError(t, err)
	_ = (*resp.AdmireComment).(*admireCommentMutationAdmireCommentAdmireCommentPayload)
}

// commentOnPost makes a GraphQL request to comment on a post
func commentOnPost(t *testing.T, ctx context.Context, c genql.Client, postID persist.DBID, comment string) {
	t.Helper()
	resp, err := commentOnPostMutation(ctx, c, postID, comment)
	require.NoError(t, err)
	_ = (*resp.CommentOnPost).(*commentOnPostMutationCommentOnPostCommentOnPostPayload)
}

func deletePost(t *testing.T, ctx context.Context, c genql.Client, postID persist.DBID) {
	t.Helper()
	resp, err := deletePostMutation(ctx, c, postID)
	require.NoError(t, err)
	_ = (*resp.DeletePost).(*deletePostMutationDeletePostDeletePostPayload)
}

func communityByAddress(t *testing.T, ctx context.Context, c genql.Client, address ChainAddressInput) persist.DBID {
	t.Helper()
	resp, err := communityByAddressQuery(ctx, c, address)
	require.NoError(t, err)
	it := (*resp.CommunityByAddress).(*communityByAddressQueryCommunityByAddressCommunity)
	posts := it.GetPosts().GetEdges()
	if len(posts) == 0 {
		t.Fatal("no posts found")
	}
	return it.Dbid
}

func contractAddressByTokenID(t *testing.T, ctx context.Context, c genql.Client, tokenID persist.DBID) string {
	t.Helper()
	resp, err := tokenByIdQuery(ctx, c, tokenID)
	require.NoError(t, err)
	it := (*resp.TokenById).(*tokenByIdQueryTokenByIdToken)
	co := it.GetContract().ContractAddress.GetAddress()
	return *co
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

// dummyToken returns a dummy token owned by the provided address
func dummyToken(ownerAddress persist.Address) common.ChainAgnosticToken {
	return dummyTokenContract(ownerAddress, "0x123")
}

// dummyTokenContract returns a dummy token owned by the provided address from the provided contract
func dummyTokenContract(ownerAddress, contractAddress persist.Address) common.ChainAgnosticToken {
	return dummyTokenIDContract(ownerAddress, contractAddress, "1")
}

// dummyTokenIDContract returns a dummy token owned by the provided address from the provided contract with the given tokenID
func dummyTokenIDContract(ownerAddress, contractAddress persist.Address, tokenID persist.HexTokenID) common.ChainAgnosticToken {
	return common.ChainAgnosticToken{
		TokenID:         tokenID,
		Quantity:        "1",
		ContractAddress: contractAddress,
		OwnerAddress:    ownerAddress,
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
func defaultHandler(t *testing.T) http.Handler {
	ctx := context.Background()
	c := server.ClientInit(ctx)
	r := newStubRecommender(t, []persist.DBID{})
	handler := server.CoreInit(ctx, c, r, newStubPersonalization(t))
	t.Cleanup(func() {
		c.Close()
	})
	return handler
}

// handlerWithProviders returns a GraphQL http.Handler
func handlerWithProviders(t *testing.T, submitter tokenmanage.Submitter, p multichain.ProviderLookup) http.Handler {
	ctx := context.Background()
	c := server.ClientInit(context.Background())
	provider := newMultichainProvider(c, submitter, p)
	t.Cleanup(c.Close)

	lock := redis.NewLockClient(redis.NewCache(redis.NotificationLockCache))
	feedCache := redis.NewCache(redis.FeedCache)
	socialCache := redis.NewCache(redis.SocialCache)
	authRefreshCache := redis.NewCache(redis.AuthTokenForceRefreshCache)
	tokenManageCache := redis.NewCache(redis.TokenManageCache)
	oneTimeLoginCache := redis.NewCache(redis.OneTimeLoginCache)
	mintLimiter := limiters.NewKeyRateLimiter(ctx, redis.NewCache(redis.MintCache), "inAppMinting", 1, time.Minute*10)

	publicapiF := func(ctx context.Context, disableDataloaderCaching bool) *publicapi.PublicAPI {
		return publicapi.NewWithMultichainProvider(
			ctx,
			false,
			c.Repos,
			c.Queries,
			c.HTTPClient,
			c.EthClient,
			c.IPFSClient,
			c.ArweaveClient,
			c.StorageClient,
			c.TaskClient,
			nil, // throttler
			c.SecretClient,
			nil,         // apqCache
			feedCache,   // feedCache
			socialCache, // socialCache
			authRefreshCache,
			tokenManageCache,  // tokenmanageCache
			oneTimeLoginCache, // oneTimeLoginCache
			c.MagicLinkClient,
			nil,         // neynar
			mintLimiter, // mintLimiter
			&provider,
		)
	}

	handlerInitF := func(r *gin.Engine) {
		server.GraphqlHandlersInit(
			r,
			c.Queries,
			c.TaskClient,
			c.PubSubClient,
			lock,             // redislock
			nil,              // apqCache
			authRefreshCache, // authRefreshCache
			newStubRecommender(t, []persist.DBID{}),
			newStubPersonalization(t),
			nil, // neynar
			publicapiF,
		)
	}

	return server.CoreInitHandlerF(ctx, handlerInitF)
}

// newMultichainProvider a new multichain provider configured with the given providers
func newMultichainProvider(c *server.Clients, submitter tokenmanage.Submitter, p multichain.ProviderLookup) multichain.Provider {
	return multichain.Provider{
		Repos:     c.Repos,
		Queries:   c.Queries,
		Chains:    p,
		Submitter: submitter,
	}
}

// defaultHandlerClient returns a GraphQL client attached to a backend GraphQL handler
func defaultHandlerClient(t *testing.T) *handlerClient {
	return customHandlerClient(t, defaultHandler(t))
}

// authedHandlerClient returns a GraphQL client with an authenticated JWT
func authedHandlerClient(t *testing.T, userID persist.DBID) *handlerClient {
	return customHandlerClient(t, defaultHandler(t), withJWTOpt(t, userID))
}

// customHandlerClient configures the client with the provided HTTP handler and client options
func customHandlerClient(t *testing.T, handler http.Handler, opts ...func(*http.Request)) *handlerClient {
	return &handlerClient{handler: handler, opts: opts, endpoint: "/glry/graphql/query"}
}

// authedServerClient provides an authenticated client to a live server
func authedServerClient(t *testing.T, host string, userID persist.DBID) *serverClient {
	return customServerClient(t, host, withJWTOpt(t, userID))
}

// customServerClient provides a client to a live server with custom options
func customServerClient(t *testing.T, host string, opts ...func(*http.Request)) *serverClient {
	return &serverClient{url: host + "/glry/graphql/query", opts: opts}
}

// withJWTOpt adds auth JWT cookies to the request headers
func withJWTOpt(t *testing.T, userID persist.DBID) func(*http.Request) {
	sessionID := persist.GenerateID()
	refreshID := persist.GenerateID().String()
	authJWT, err := auth.GenerateAuthToken(context.Background(), userID, sessionID, refreshID, []persist.Role{})
	require.NoError(t, err)
	refreshJWT, _, err := auth.GenerateRefreshToken(context.Background(), refreshID, "", userID, sessionID)
	require.NoError(t, err)
	return func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: auth.AuthCookieKey, Value: authJWT})
		r.AddCookie(&http.Cookie{Name: auth.RefreshCookieKey, Value: refreshJWT})
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
	cookies := r.Cookies()
	for i := len(cookies) - 1; i >= 0; i-- {
		c := cookies[i]
		if c.Name == name {
			return c.Value
		}
	}
	require.NoError(t, fmt.Errorf("%s not set as a cookie", name))
	return ""
}
