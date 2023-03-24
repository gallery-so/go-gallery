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

	"github.com/Khan/genqlient/graphql"
	genql "github.com/Khan/genqlient/graphql"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
		{
			title:    "test syncing tokens",
			run:      testTokenSyncs,
			fixtures: []fixture{useDefaultEnv, usePostgres, useRedis, useTokenQueue, useTokenProcessing},
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
		{title: "should get viewer suggested users", run: testSuggestedUsersForViewer},
		{title: "should add a wallet", run: testAddWallet},
		{title: "should remove a wallet", run: testRemoveWallet},
		{title: "should create a collection", run: testCreateCollection},
		{title: "views from multiple users are rolled up", run: testViewsAreRolledUp},
		{title: "update gallery and create a feed event", run: testUpdateGalleryWithPublish},
		{title: "update gallery and ensure name still gets set when not sent in update", run: testUpdateGalleryWithNoNameChange},
		{title: "update gallery with a new collection", run: testUpdateGalleryWithNewCollection},
		{title: "should get trending users", run: testTrendingUsers, fixtures: []fixture{usePostgres, useRedis}},
		{title: "should get trending feed events", run: testTrendingFeedEvents},
		{title: "should delete collection in gallery update", run: testUpdateGalleryDeleteCollection},
		{title: "should update user experiences", run: testUpdateUserExperiences},
		{title: "should create gallery", run: testCreateGallery},
		{title: "should move collection to new gallery", run: testMoveCollection},
		{title: "should connect social account", run: testConnectSocialAccount},
	}
	for _, test := range tests {
		t.Run(test.title, testWithFixtures(test.run, test.fixtures...))
	}
}

func testTokenSyncs(t *testing.T) {
	tests := []testCase{
		{title: "should sync new tokens", run: testSyncNewTokens},
		{title: "should submit new tokens to tokenprocessing", run: testSyncOnlySubmitsNewTokens},
		{title: "should not submit old tokens to tokenprocessing", run: testSyncSkipsSubmittingOldTokens},
		{title: "should delete old tokens", run: testSyncDeletesOldTokens},
		{title: "should combine all tokens from providers", run: testSyncShouldCombineProviders},
		{title: "should merge duplicates within provider", run: testSyncShouldMergeDuplicatesInProvider},
		{title: "should merge duplicates across providers", run: testSyncShouldMergeDuplicatesAcrossProviders},
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

	response, err := createUserMutation(context.Background(), c, authMechanismInput(nonceF.Wallet, nonceF.Nonce),
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

	response, err := userByAddressQuery(context.Background(), c, chainAddressInput(userF.Wallet.Address))

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

func testSuggestedUsersForViewer(t *testing.T) {
	userF := newUserFixture(t)
	userA := newUserFixture(t)
	userB := newUserFixture(t)
	userC := newUserFixture(t)
	ctx := context.Background()
	clients := server.ClientInit(ctx)
	provider := server.NewMultichainProvider(clients)
	recommender := newStubRecommender(t, []persist.DBID{
		userA.ID,
		userB.ID,
		userC.ID,
	})
	handler := server.CoreInit(clients, provider, recommender)
	c := customHandlerClient(t, handler, withJWTOpt(t, userF.ID))

	response, err := viewerQuery(ctx, c)
	require.NoError(t, err)

	payload, _ := (*response.Viewer).(*viewerQueryViewer)
	suggested := payload.GetSuggestedUsers().GetEdges()
	assert.Len(t, suggested, 3)
}

func testAddWallet(t *testing.T) {
	userF := newUserFixture(t)
	walletToAdd := newWallet(t)
	ctx := context.Background()
	c := authedHandlerClient(t, userF.ID)
	nonce := newNonce(t, ctx, c, walletToAdd)

	response, err := addUserWalletMutation(ctx, c, chainAddressInput(walletToAdd.Address), authMechanismInput(walletToAdd, nonce))

	require.NoError(t, err)
	payload, _ := (*response.AddUserWallet).(*addUserWalletMutationAddUserWalletAddUserWalletPayload)
	wallets := payload.Viewer.User.Wallets
	assert.Equal(t, walletToAdd.Address, *wallets[len(wallets)-1].ChainAddress.Address)
	assert.Equal(t, Chain("Ethereum"), *wallets[len(wallets)-1].ChainAddress.Chain)
	assert.Len(t, wallets, 2)
}

func testRemoveWallet(t *testing.T) {
	userF := newUserFixture(t)
	walletToRemove := newWallet(t)
	ctx := context.Background()
	c := authedHandlerClient(t, userF.ID)
	nonce := newNonce(t, ctx, c, walletToRemove)
	addResponse, err := addUserWalletMutation(ctx, c, chainAddressInput(walletToRemove.Address), authMechanismInput(walletToRemove, nonce))
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
	nonce := newNonce(t, ctx, c, userF.Wallet)

	response, err := loginMutation(ctx, c, authMechanismInput(userF.Wallet, nonce))

	require.NoError(t, err)
	payload, _ := (*response.Login).(*loginMutationLoginLoginPayload)
	assert.NotEmpty(t, readCookie(t, c.response, auth.JWTCookieKey))
	assert.Equal(t, userF.Username, *payload.Viewer.User.Username)
	assert.Equal(t, userF.ID, payload.Viewer.User.Dbid)
}

func testLogout(t *testing.T) {
	userF := newUserFixture(t)
	c := authedHandlerClient(t, userF.ID)

	response, err := logoutMutation(context.Background(), c)

	require.NoError(t, err)
	assert.Empty(t, readCookie(t, c.response, auth.JWTCookieKey))
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
	node := vPayload.User.Feed.Edges[0].Node
	assert.NotNil(t, node)
	feedEvent := (*node).(*viewerQueryViewerUserGalleryUserFeedFeedConnectionEdgesFeedEdgeNodeFeedEvent)
	assert.Equal(t, "newCaption", *feedEvent.Caption)
	edata := *(*feedEvent.EventData).(*viewerQueryViewerUserGalleryUserFeedFeedConnectionEdgesFeedEdgeNodeFeedEventEventDataGalleryUpdatedFeedEventData)
	assert.EqualValues(t, persist.ActionGalleryUpdated, *edata.Action)

	nameIncluded := false
	descIncluded := false

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
		if persist.Action(*ac) == persist.ActionGalleryInfoUpdated {
			ca := c.(*viewerQueryViewerUserGalleryUserFeedFeedConnectionEdgesFeedEdgeNodeFeedEventEventDataGalleryUpdatedFeedEventDataSubEventDatasGalleryInfoUpdatedFeedEventData)
			if ca.NewDescription != nil {
				assert.Equal(t, "newDesc", *ca.NewDescription)
				descIncluded = true
			}
			if ca.NewName != nil {
				assert.Equal(t, "newName", *ca.NewName)
				nameIncluded = true
			}
		}
	}

	assert.True(t, nameIncluded)
	assert.True(t, descIncluded)
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
	dc := defaultHandlerClient(t)

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

	userResp, err := userByIdQuery(context.Background(), dc, userF.ID)
	require.NoError(t, err)
	userPayload := (*userResp.UserById).(*userByIdQueryUserByIdGalleryUser)
	assert.Nil(t, userPayload.SocialAccounts.Twitter)

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
	serverF := newServerFixture(t)
	bob := newUserWithFeedEventsFixture(t)
	alice := newUserWithFeedEventsFixture(t)
	dave := newUserWithFeedEventsFixture(t)
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
	userF := newUserWithFeedEventsFixture(t)
	c := authedHandlerClient(t, userF.ID)
	admireFeedEvent(t, ctx, c, userF.FeedEventIDs[1])
	commentOnFeedEvent(t, ctx, c, userF.FeedEventIDs[1], "a")
	commentOnFeedEvent(t, ctx, c, userF.FeedEventIDs[1], "b")
	commentOnFeedEvent(t, ctx, c, userF.FeedEventIDs[1], "c")
	admireFeedEvent(t, ctx, c, userF.FeedEventIDs[0])
	commentOnFeedEvent(t, ctx, c, userF.FeedEventIDs[0], "a")
	commentOnFeedEvent(t, ctx, c, userF.FeedEventIDs[0], "b")
	admireFeedEvent(t, ctx, c, userF.FeedEventIDs[2])
	expected := []persist.DBID{
		userF.FeedEventIDs[2],
		userF.FeedEventIDs[0],
		userF.FeedEventIDs[1],
	}

	actual := trendingFeedEvents(t, ctx, c, 10)

	assert.Equal(t, expected, actual)
}

func testSyncNewTokens(t *testing.T) {
	userF := newUserFixture(t)
	provider := defaultStubProvider(userF.Wallet.Address)
	h := handlerWithProviders(t, sendTokensNOOP, provider)
	c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))
	ctx := context.Background()

	t.Run("should sync new tokens", func(t *testing.T) {
		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		require.NoError(t, err)
		payload := (*response.SyncTokens).(*syncTokensMutationSyncTokensSyncTokensPayload)
		assert.Len(t, payload.Viewer.User.Tokens, len(provider.Tokens))
	})

	t.Run("should not duplicate tokens from repeat syncs", func(t *testing.T) {
		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		require.NoError(t, err)
		payload := (*response.SyncTokens).(*syncTokensMutationSyncTokensSyncTokensPayload)
		assert.Len(t, payload.Viewer.User.Tokens, len(provider.Tokens))
	})
}

func testSyncOnlySubmitsNewTokens(t *testing.T) {
	userF := newUserFixture(t)
	provider := defaultStubProvider(userF.Wallet.Address)
	tokenRecorder := sendTokensRecorder{}
	h := handlerWithProviders(t, tokenRecorder.Send, provider)
	c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))
	tokenRecorder.On("Send", mock.Anything, mock.Anything).Times(1).Return(nil)

	_, err := syncTokensMutation(context.Background(), c, []Chain{ChainEthereum})

	require.NoError(t, err)
	tokenRecorder.AssertExpectations(t)
	assert.Len(t, tokenRecorder.Tasks[0].TokenIDs, len(provider.Tokens))
}

func testSyncSkipsSubmittingOldTokens(t *testing.T) {
	userF := newUserWithTokensFixture(t)
	tokenRecorder := sendTokensRecorder{}
	h := handlerWithProviders(t, tokenRecorder.Send, defaultStubProvider(userF.Wallet.Address))
	c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

	_, err := syncTokensMutation(context.Background(), c, []Chain{ChainEthereum})

	require.NoError(t, err)
	tokenRecorder.AssertNotCalled(t, "Send", mock.Anything, mock.Anything)
}

func testSyncDeletesOldTokens(t *testing.T) {
	userF := newUserWithTokensFixture(t)
	provider := newStubProvider(withContractTokens(multichain.ChainAgnosticContract{
		Address: "0x1337",
		Name:    "someContract",
	}, userF.Wallet.Address, 4))
	h := handlerWithProviders(t, sendTokensNOOP, provider)
	c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

	response, err := syncTokensMutation(context.Background(), c, []Chain{ChainEthereum})

	assertSyncedTokens(t, response, err, 4)
}

func testSyncShouldCombineProviders(t *testing.T) {
	userF := newUserFixture(t)
	providerA := newStubProvider(withContractTokens(multichain.ChainAgnosticContract{
		Address: "0x1337",
		Name:    "someContract",
	}, userF.Wallet.Address, 4))
	providerB := newStubProvider(withContractTokens(multichain.ChainAgnosticContract{
		Address: "0x1234",
		Name:    "anotherContract",
	}, userF.Wallet.Address, 2))
	h := handlerWithProviders(t, sendTokensNOOP, providerA, providerB)
	c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

	response, err := syncTokensMutation(context.Background(), c, []Chain{ChainEthereum})

	assertSyncedTokens(t, response, err, len(providerA.Tokens)+len(providerB.Tokens))
}

func testSyncShouldMergeDuplicatesInProvider(t *testing.T) {
	userF := newUserFixture(t)
	token := defaultToken(userF.Wallet.Address)
	provider := newStubProvider(withTokens([]multichain.ChainAgnosticToken{token, token}))
	h := handlerWithProviders(t, sendTokensNOOP, provider)
	c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

	response, err := syncTokensMutation(context.Background(), c, []Chain{ChainEthereum})

	assertSyncedTokens(t, response, err, 1)
}

func testSyncShouldMergeDuplicatesAcrossProviders(t *testing.T) {
	userF := newUserFixture(t)
	token := defaultToken(userF.Wallet.Address)
	providerA := newStubProvider(withTokens([]multichain.ChainAgnosticToken{token}))
	providerB := newStubProvider(withTokens([]multichain.ChainAgnosticToken{token}))
	h := handlerWithProviders(t, sendTokensNOOP, providerA, providerB)
	c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

	response, err := syncTokensMutation(context.Background(), c, []Chain{ChainEthereum})

	assertSyncedTokens(t, response, err, 1)
}

func testSyncShouldProcessMedia(t *testing.T) {
	metadataServer := newMetadataServerFixture(t)

	patchMetadata := func(t *testing.T, ctx context.Context, address, endpoint string) http.Handler {
		contract := multichain.ChainAgnosticContract{Address: "0x123", Name: "testContract"}
		provider := newStubProvider(
			withContractTokens(contract, address, 1),
			withFetchMetadata(fetchFromDummyEndpoint(metadataServer.URL, endpoint)),
		)
		clients := server.ClientInit(ctx)
		mc := newMultichainProvider(clients, sendTokensNOOP, []any{provider})
		t.Cleanup(clients.Close)
		return handlerWithProviders(t, sendTokensToTokenProcessing(clients, &mc), provider)
	}

	t.Run("sync should process image", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/image")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaImageMedia)
		assert.Equal(t, string(persist.MediaTypeImage), *media.MediaType)
		assert.NotEmpty(t, media.MediaURL)
	})

	t.Run("sync should process video", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/video")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaVideoMedia)
		assert.Equal(t, string(persist.MediaTypeVideo), *media.MediaType)
		assert.NotEmpty(t, media.MediaURL)
	})

	t.Run("sync should process iframe", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/iframe")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaHtmlMedia)
		assert.Equal(t, string(persist.MediaTypeHTML), *media.MediaType)
		assert.NotEmpty(t, media.MediaURL)
	})

	t.Run("sync should process gif", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/gif")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaGIFMedia)
		assert.Equal(t, string(persist.MediaTypeGIF), *media.MediaType)
		assert.NotEmpty(t, media.MediaURL)
	})

	t.Run("sync should process bad metadata", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/bad")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaInvalidMedia)
		assert.Equal(t, "", *media.MediaType)
		assert.Empty(t, media.MediaURL)
	})

	t.Run("sync should process missing metadata", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/notfound")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaInvalidMedia)
		assert.Equal(t, "", *media.MediaType)
		assert.Empty(t, media.MediaURL)
	})

	t.Run("sync should process bad media", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/media/bad")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaUnknownMedia)
		assert.Equal(t, string(persist.MediaTypeUnknown), *media.MediaType)
		assert.NotEmpty(t, media.MediaURL)
	})

	t.Run("sync should process missing media", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/media/notfound")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaUnknownMedia)
		assert.Equal(t, string(persist.MediaTypeUnknown), *media.MediaType)
		assert.NotEmpty(t, media.MediaURL)
	})

	t.Run("sync should process svg", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/svg")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaImageMedia)
		assert.Equal(t, string(persist.MediaTypeSVG), *media.MediaType)
		assert.NotEmpty(t, media.MediaURL)
	})

	t.Run("sync should process base64svg", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/base64svg")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaImageMedia)
		assert.Equal(t, string(persist.MediaTypeSVG), *media.MediaType)
		assert.NotEmpty(t, media.MediaURL)
	})

	t.Run("sync should process base64", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/base64")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaImageMedia)
		assert.Equal(t, *media.MediaType, string(persist.MediaTypeSVG))
		assert.NotEmpty(t, media.MediaURL)
	})

	t.Run("sync should process ipfs", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/media/ipfs")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaImageMedia)
		assert.Equal(t, *media.MediaType, string(persist.MediaTypeImage))
		assert.NotEmpty(t, media.MediaURL)
	})

	t.Run("sync should process bad dns", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/media/dnsbad")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaImageMedia)
		assert.Equal(t, string(persist.MediaTypeImage), *media.MediaType)
		assert.NotEmpty(t, media.MediaURL)
	})

	t.Run("sync should process different keyword", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/differentkeyword")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaImageMedia)
		assert.Equal(t, string(persist.MediaTypeImage), *media.MediaType)
		assert.NotEmpty(t, media.MediaURL)
	})

	t.Run("sync should process wrong keyword", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/wrongkeyword")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaVideoMedia)
		assert.Equal(t, string(persist.MediaTypeVideo), *media.MediaType)
		assert.NotEmpty(t, media.MediaURL)
	})

	t.Run("sync should process animation", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/animation")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaGltfMedia)
		assert.Equal(t, string(persist.MediaTypeAnimation), *media.MediaType)
		assert.NotEmpty(t, media.MediaURL)
	})

	t.Run("sync should process pdf", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/pdf")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaPdfMedia)
		assert.Equal(t, string(persist.MediaTypePDF), *media.MediaType)
		assert.NotEmpty(t, media.MediaURL)
	})

	t.Run("sync should process text", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/text")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaTextMedia)
		assert.Equal(t, string(persist.MediaTypeText), *media.MediaType)
		assert.NotEmpty(t, media.MediaURL)
	})

	t.Run("sync should process bad image", func(t *testing.T) {
		ctx := context.Background()
		userF := newUserFixture(t)
		h := patchMetadata(t, ctx, userF.Wallet.Address, "/metadata/badimage")
		c := customHandlerClient(t, h, withJWTOpt(t, userF.ID))

		response, err := syncTokensMutation(ctx, c, []Chain{ChainEthereum})

		tokens := assertSyncedTokens(t, response, err, 1)
		media := (*tokens[0].Media).(*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensTokenMediaImageMedia)
		assert.Equal(t, string(persist.MediaTypeImage), *media.MediaType)
		assert.NotEmpty(t, media.MediaURL)
	})
}

func assertSyncedTokens(t *testing.T, response *syncTokensMutationResponse, err error, expectedLen int) []*syncTokensMutationSyncTokensSyncTokensPayloadViewerUserGalleryUserTokensToken {
	t.Helper()
	require.NoError(t, err)
	payload := (*response.SyncTokens).(*syncTokensMutationSyncTokensSyncTokensPayload)
	assert.Len(t, payload.Viewer.User.Tokens, expectedLen)
	return payload.Viewer.User.Tokens
}

// authMechanismInput signs a nonce with an ethereum wallet
func authMechanismInput(w wallet, nonce string) AuthMechanism {
	return AuthMechanism{
		Eoa: &EoaAuth{
			Nonce:     nonce,
			Signature: w.Sign(nonce),
			ChainPubKey: ChainPubKeyInput{
				PubKey: w.Address,
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
	Address string
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
		Address: address,
	}
}

func newNonce(t *testing.T, ctx context.Context, c graphql.Client, w wallet) string {
	t.Helper()
	response, err := getAuthNonceMutation(ctx, c, chainAddressInput(w.Address))
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

// defaultToken returns a dummy token owned by the provided address
func defaultToken(address string) multichain.ChainAgnosticToken {
	return multichain.ChainAgnosticToken{
		Name:            "testToken1",
		TokenID:         "1",
		Quantity:        "1",
		ContractAddress: "0x123",
		OwnerAddress:    persist.Address(address),
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
	c := server.ClientInit(context.Background())
	p := server.NewMultichainProvider(c)
	r := newStubRecommender(t, []persist.DBID{})
	handler := server.CoreInit(c, p, r)
	t.Cleanup(c.Close)
	return handler
}

// handlerWithProviders returns a GraphQL http.Handler
func handlerWithProviders(t *testing.T, sendTokens multichain.SendTokens, providers ...any) http.Handler {
	c := server.ClientInit(context.Background())
	provider := newMultichainProvider(c, sendTokens, providers)
	recommender := newStubRecommender(t, []persist.DBID{})
	t.Cleanup(c.Close)
	return server.CoreInit(c, &provider, recommender)
}

// newMultichainProvider a new multichain provider configured with the given providers
func newMultichainProvider(c *server.Clients, sendToken multichain.SendTokens, providers []any) multichain.Provider {
	return multichain.Provider{
		Repos:      c.Repos,
		Queries:    c.Queries,
		Chains:     map[persist.Chain][]any{persist.ChainETH: providers},
		SendTokens: sendToken,
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
