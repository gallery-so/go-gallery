package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/assert"
)

func TestUpdateCollectionNameByID_Success(t *testing.T) {
	assert := setupTest(t)

	// seed DB with collection
	collID, err := persist.CollCreate(context.Background(), &persist.CollectionDB{
		Name:        "very cool collection",
		OwnerUserID: tc.user1.id,
	}, tc.r)
	assert.Nil(err)

	// build update request body
	update := collectionUpdateInfoByIDInput{Name: "new coll name", ID: collID}
	resp := updateCollectionInfoRequest(assert, update, tc.user1.jwt)
	assertValidResponse(assert, resp)

	// retrieve updated nft
	resp, err = http.Get(fmt.Sprintf("%s/collections/get?id=%s", tc.serverURL, collID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	type CollectionGetResponse struct {
		Collection *persist.Collection `json:"collection"`
		Error      string              `json:"error"`
	}
	// ensure nft was updated
	body := CollectionGetResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.NotNil(body.Collection)
	assert.Empty(body.Error)
	assert.Equal(update.Name, body.Collection.Name)
}

func TestCreateCollection_Success(t *testing.T) {
	assert := setupTest(t)

	nftIDs := seedTokens(assert)

	gid, err := persist.GalleryCreate(context.Background(), &persist.GalleryDB{OwnerUserID: tc.user1.id}, tc.r)
	assert.Nil(err)

	input := collectionCreateInput{GalleryID: gid, Nfts: nftIDs}
	resp := createCollectionRequest(assert, input, tc.user1.jwt)
	assertValidResponse(assert, resp)

	type CreateResp struct {
		ID    persist.DBID `json:"collection_id"`
		Error string       `json:"error"`
	}

	createResp := &CreateResp{}
	err = util.UnmarshallBody(createResp, resp.Body)
	assert.Nil(err)
	assert.Empty(createResp.Error)

	// retrieve updated nft
	resp, err = http.Get(fmt.Sprintf("%s/collections/get?id=%s", tc.serverURL, createResp.ID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	type CollectionGetResponse struct {
		Collection *persist.Collection `json:"collection"`
		Error      string              `json:"error"`
	}
	// ensure nft was updated
	body := CollectionGetResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.NotNil(body.Collection)
	assert.Len(body.Collection.Nfts, 3)
	assert.Empty(body.Error)

	gallery, err := persist.GalleryGetByID(context.Background(), gid, true, tc.r)
	fmt.Println(gallery[0])
	assert.Nil(err)
	assert.Len(gallery[0].Collections, 1)
}

func TestGetUnassignedCollection_Success(t *testing.T) {
	assert := setupTest(t)

	nftIDs := seedTokens(assert)
	// seed DB with collection
	_, err := persist.CollCreate(context.Background(), &persist.CollectionDB{
		Name:        "very cool collection",
		OwnerUserID: tc.user1.id,
		Nfts:        nftIDs[:2],
	}, tc.r)
	assert.Nil(err)

	resp := getUnassignedNFTsRequest(assert, tc.user1.id)
	assertValidResponse(assert, resp)

	type NftsResponse struct {
		Nfts  []*persist.Token `json:"nfts"`
		Error string           `json:"error"`
	}
	// ensure nft was updated
	body := NftsResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Len(body.Nfts, 1)
	assert.Empty(body.Error)
}

func TestDeleteCollection_Success(t *testing.T) {
	assert := setupTest(t)

	collID := createCollectionInDbForUserID(assert, "COLLECTION NAME", tc.user1.id)
	verifyCollectionExistsInDbForID(assert, collID)

	resp := sendDeleteRequest(assert, collectionDeleteInput{ID: collID}, tc.user1)

	assertValidResponse(assert, resp)

	// Assert that the collection was deleted
	collectionsAfterDelete, err := persist.CollGetByID(context.Background(), collID, false, tc.r)
	assert.Nil(err)

	assert.True(len(collectionsAfterDelete) == 0)
}

func TestDeleteCollection_Failure_Unauthenticated(t *testing.T) {
	assert := setupTest(t)

	collID := createCollectionInDbForUserID(assert, "COLLECTION NAME", tc.user1.id)
	verifyCollectionExistsInDbForID(assert, collID)

	resp := sendDeleteRequest(assert, collectionDeleteInput{ID: collID}, nil)

	assert.Equal(401, resp.StatusCode)
}

func TestDeleteCollection_Failure_DifferentUsersCollection(t *testing.T) {
	assert := setupTest(t)

	collID := createCollectionInDbForUserID(assert, "COLLECTION NAME", tc.user1.id)
	verifyCollectionExistsInDbForID(assert, collID)

	resp := sendDeleteRequest(assert, collectionDeleteInput{ID: collID}, tc.user2)
	assert.Equal(404, resp.StatusCode)
}

func TestGetHiddenCollections_Success(t *testing.T) {
	assert := setupTest(t)

	nftIDs := seedTokens(assert)

	_, err := persist.CollCreate(context.Background(), &persist.CollectionDB{
		Name:        "very cool collection",
		OwnerUserID: tc.user1.id,
		Nfts:        nftIDs,
		Hidden:      true,
	}, tc.r)
	assert.Nil(err)

	resp := sendCollUserGetRequest(assert, string(tc.user1.id), tc.user1)

	type CollectionsResponse struct {
		Collections []*persist.Collection `json:"collections"`
		Error       string                `json:"error"`
	}

	body := CollectionsResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Len(body.Collections, 1)
	assert.Empty(body.Error)
}

func TestGetNoHiddenCollections_Success(t *testing.T) {
	assert := setupTest(t)

	nftIDs := seedTokens(assert)

	_, err := persist.CollCreate(context.Background(), &persist.CollectionDB{
		Name:        "very cool collection",
		OwnerUserID: tc.user1.id,
		Nfts:        nftIDs[0:1],
		Hidden:      false,
	}, tc.r)
	_, err = persist.CollCreate(context.Background(), &persist.CollectionDB{
		Name:        "very cool collection",
		OwnerUserID: tc.user1.id,
		Nfts:        nftIDs[1:],
		Hidden:      true,
	}, tc.r)
	assert.Nil(err)

	resp := sendCollUserGetRequest(assert, string(tc.user1.id), tc.user2)

	type CollectionsResponse struct {
		Collections []*persist.Collection `json:"collections"`
		Error       string                `json:"error"`
	}

	body := CollectionsResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Len(body.Collections, 1)
	assert.Empty(body.Error)
}

func TestCreateCollectionWithUsedNFT_Success(t *testing.T) {
	assert := setupTest(t)

	nftIDs := seedTokens(assert)

	preCollID, err := persist.CollCreate(context.Background(), &persist.CollectionDB{Name: "test", Nfts: nftIDs, OwnerUserID: tc.user1.id}, tc.r)
	gid, err := persist.GalleryCreate(context.Background(), &persist.GalleryDB{Collections: []persist.DBID{preCollID}, OwnerUserID: tc.user1.id}, tc.r)

	input := collectionCreateInput{GalleryID: gid, Nfts: nftIDs[0:2]}
	resp := createCollectionRequest(assert, input, tc.user1.jwt)
	assertValidResponse(assert, resp)

	resp, err = http.Get(fmt.Sprintf("%s/collections/get?id=%s", tc.serverURL, preCollID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	type CollectionGetResponse struct {
		Collection *persist.Collection `json:"collection"`
		Error      string              `json:"error"`
	}
	// ensure collection was updated
	body := CollectionGetResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.NotNil(body.Collection)
	assert.Len(body.Collection.Nfts, 1)
	assert.Empty(body.Error)

}

func TestUpdateCollectionNftsOrder_Success(t *testing.T) {
	assert := setupTest(t)

	nftIDs := seedTokens(assert)

	collID, err := persist.CollCreate(context.Background(), &persist.CollectionDB{
		Name:        "very cool collection",
		OwnerUserID: tc.user1.id,
		Nfts:        nftIDs,
	}, tc.r)
	assert.Nil(err)

	temp := nftIDs[1]
	nftIDs[1] = nftIDs[2]
	nftIDs[2] = temp

	update := collectionUpdateNftsByIDinput{ID: collID, Nfts: nftIDs}
	resp := updateCollectionNftsRequest(assert, update, tc.user1.jwt)
	assertValidResponse(assert, resp)

	errResp := util.ErrorResponse{}
	util.UnmarshallBody(&errResp, resp.Body)
	assert.Empty(errResp.Error)

	// retrieve updated nft
	resp, err = http.Get(fmt.Sprintf("%s/collections/get?id=%s", tc.serverURL, collID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	type CollectionGetResponse struct {
		Collection *persist.Collection `json:"collection"`
		Error      string              `json:"error"`
	}
	// ensure nft was updated
	body := CollectionGetResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.NotNil(body.Collection)
	assert.Empty(body.Error)
	assert.Equal(update.Nfts[1], body.Collection.Nfts[1].ID)
}

func verifyCollectionExistsInDbForID(assert *assert.Assertions, collID persist.DBID) {
	collectionsBeforeDelete, err := persist.CollGetByID(context.Background(), collID, false, tc.r)
	assert.Nil(err)
	assert.Equal(collectionsBeforeDelete[0].ID, collID)
}

func sendDeleteRequest(assert *assert.Assertions, requestBody interface{}, authenticatedUser *TestUser) *http.Response {
	data, err := json.Marshal(requestBody)
	assert.Nil(err)

	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/collections/delete", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)

	if authenticatedUser != nil {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authenticatedUser.jwt))
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)

	return resp
}

func getUnassignedNFTsRequest(assert *assert.Assertions, userID persist.DBID) *http.Response {
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/nfts/get_unassigned?user_id=%s&skip_cache=false", tc.serverURL, userID),
		nil)
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	return resp
}

func sendCollUserGetRequest(assert *assert.Assertions, forUserID string, authenticatedUser *TestUser) *http.Response {

	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/collections/user_get?user_id=%s", tc.serverURL, forUserID),
		nil)
	assert.Nil(err)

	if authenticatedUser != nil {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authenticatedUser.jwt))
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertValidResponse(assert, resp)

	return resp
}

func createCollectionRequest(assert *assert.Assertions, input collectionCreateInput, jwt string) *http.Response {
	data, err := json.Marshal(input)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/collections/create", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	return resp
}

func updateCollectionInfoRequest(assert *assert.Assertions, input collectionUpdateInfoByIDInput, jwt string) *http.Response {
	data, err := json.Marshal(input)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/collections/update/info", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	return resp
}
func updateCollectionNftsRequest(assert *assert.Assertions, input collectionUpdateNftsByIDinput, jwt string) *http.Response {
	data, err := json.Marshal(input)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/collections/update/nfts", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	return resp
}

func createCollectionInDbForUserID(assert *assert.Assertions, collectionName string, userID persist.DBID) persist.DBID {
	collID, err := persist.CollCreate(context.Background(), &persist.CollectionDB{
		Name:        collectionName,
		OwnerUserID: userID,
	}, tc.r)
	assert.Nil(err)

	return collID
}

func seedTokens(assert *assert.Assertions) []persist.DBID {
	nfts := []*persist.Token{
		{OwnerUserID: tc.user1.id, CollectorsNote: "asd", OwnerAddress: tc.user1.address},
		{OwnerUserID: tc.user1.id, CollectorsNote: "bbb", OwnerAddress: tc.user1.address},
		{OwnerUserID: tc.user1.id, CollectorsNote: "wowowowow", OwnerAddress: tc.user1.address},
	}
	nftIDs, err := persist.TokenCreateBulk(context.Background(), nfts, tc.r)
	assert.Nil(err)
	return nftIDs
}
