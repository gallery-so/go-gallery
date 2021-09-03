package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/stretchr/testify/assert"
)

func TestUpdateCollectionNameByID_Success(t *testing.T) {
	assert := assert.New(t)

	// seed DB with collection
	collID, err := persist.CollCreate(context.Background(), &persist.CollectionDB{
		Name:        "very cool collection",
		OwnerUserID: tc.user1.id,
	}, tc.r)
	assert.Nil(err)

	// build update request body
	type Update struct {
		ID             persist.DBID `json:"id"`
		Name           string       `json:"name"`
		CollectorsNote string       `json:"collectors_note"`
	}
	update := Update{Name: "new coll name", ID: collID}
	data, err := json.Marshal(update)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/collections/update/info", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertValidResponse(assert, resp)

	// retrieve updated nft
	resp, err = http.Get(fmt.Sprintf("%s/collections/get?id=%s", tc.serverURL, collID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	type CollectionGetResponse struct {
		Collections []*persist.Collection `json:"collections"`
		Error       string                `json:"error"`
	}
	// ensure nft was updated
	body := CollectionGetResponse{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Len(body.Collections, 1)
	assert.Empty(body.Error)
	assert.Equal(update.Name, body.Collections[0].Name)
}

func TestCreateCollection_Success(t *testing.T) {
	assert := assert.New(t)

	nfts := []*persist.Nft{
		{Description: "asd", OwnerUserID: tc.user1.id, CollectorsNote: "asd", OwnerAddress: tc.user1.address},
		{Description: "bbb", OwnerUserID: tc.user1.id, CollectorsNote: "bbb", OwnerAddress: tc.user1.address},
		{Description: "wowowowow", OwnerUserID: tc.user1.id, CollectorsNote: "wowowowow", OwnerAddress: tc.user1.address},
	}
	nftIDs, err := persist.NftCreateBulk(context.Background(), nfts, tc.r)
	gid, err := persist.GalleryCreate(context.Background(), &persist.GalleryDB{OwnerUserID: tc.user1.id}, tc.r)

	input := collectionCreateInput{GalleryID: gid, Nfts: nftIDs}
	data, err := json.Marshal(input)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/collections/create", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertValidResponse(assert, resp)

	type CreateResp struct {
		ID    persist.DBID `json:"collection_id"`
		Error string       `json:"error"`
	}

	createResp := &CreateResp{}
	err = runtime.UnmarshallBody(createResp, resp.Body, tc.r)
	assert.Nil(err)
	assert.Empty(createResp.Error)

	// retrieve updated nft
	resp, err = http.Get(fmt.Sprintf("%s/collections/get?id=%s", tc.serverURL, createResp.ID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	type CollectionGetResponse struct {
		Collections []*persist.Collection `json:"collections"`
		Error       string                `json:"error"`
	}
	// ensure nft was updated
	body := CollectionGetResponse{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Len(body.Collections, 1)
	assert.Len(body.Collections[0].Nfts, 3)
	assert.Empty(body.Error)

	gallery, err := persist.GalleryGetByID(context.Background(), gid, true, tc.r)
	fmt.Println(gallery[0])
	assert.Nil(err)
	assert.Len(gallery[0].Collections, 1)
}

func TestGetUnassignedCollection_Success(t *testing.T) {
	assert := assert.New(t)

	nfts := []*persist.Nft{
		{Description: "asd", OwnerUserID: tc.user1.id, CollectorsNote: "asd", OwnerAddress: tc.user1.address},
		{Description: "bbb", OwnerUserID: tc.user1.id, CollectorsNote: "bbb", OwnerAddress: tc.user1.address},
		{Description: "wowowowow", OwnerUserID: tc.user1.id, CollectorsNote: "wowowowow", OwnerAddress: tc.user1.address},
	}
	nftIDs, err := persist.NftCreateBulk(context.Background(), nfts, tc.r)
	// seed DB with collection
	_, err = persist.CollCreate(context.Background(), &persist.CollectionDB{
		Name:        "very cool collection",
		OwnerUserID: tc.user1.id,
		Nfts:        nftIDs[:2],
	}, tc.r)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/nfts/get_unassigned?user_id=%s&skip_cache=false", tc.serverURL, tc.user1.id),
		nil)
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertValidResponse(assert, resp)

	type NftsResponse struct {
		Nfts  []*persist.Nft `json:"nfts"`
		Error string         `json:"error"`
	}
	// ensure nft was updated
	body := NftsResponse{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Len(body.Nfts, 1)
	assert.Empty(body.Error)
}

func TestDeleteCollection_Success(t *testing.T) {
	assert := assert.New(t)

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
	assert := assert.New(t)

	collID := createCollectionInDbForUserID(assert, "COLLECTION NAME", tc.user1.id)
	verifyCollectionExistsInDbForID(assert, collID)

	resp := sendDeleteRequest(assert, collectionDeleteInput{ID: collID}, nil)

	assert.Equal(401, resp.StatusCode)
}

func TestDeleteCollection_Failure_DifferentUsersCollection(t *testing.T) {
	assert := assert.New(t)

	collID := createCollectionInDbForUserID(assert, "COLLECTION NAME", tc.user1.id)
	verifyCollectionExistsInDbForID(assert, collID)

	resp := sendDeleteRequest(assert, collectionDeleteInput{ID: collID}, tc.user2)
	assert.Equal(404, resp.StatusCode)
}

func TestGetHiddenCollections_Success(t *testing.T) {
	assert := assert.New(t)

	nfts := []*persist.Nft{
		{Description: "asd", OwnerUserID: tc.user1.id, CollectorsNote: "asd", OwnerAddress: tc.user1.address},
		{Description: "bbb", OwnerUserID: tc.user1.id, CollectorsNote: "bbb", OwnerAddress: tc.user1.address},
		{Description: "wowowowow", OwnerUserID: tc.user1.id, CollectorsNote: "wowowowow", OwnerAddress: tc.user1.address},
	}
	nftIDs, err := persist.NftCreateBulk(context.Background(), nfts, tc.r)

	_, err = persist.CollCreate(context.Background(), &persist.CollectionDB{
		Name:        "very cool collection",
		OwnerUserID: tc.user1.id,
		Nfts:        nftIDs,
		Hidden:      true,
	}, tc.r)
	assert.Nil(err)

	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/collections/user_get?user_id=%s", tc.serverURL, tc.user1.id),
		nil)
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertValidResponse(assert, resp)

	type CollectionsResponse struct {
		Collections []*persist.Collection `json:"collections"`
		Error       string                `json:"error"`
	}

	body := CollectionsResponse{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Len(body.Collections, 1)
	assert.Empty(body.Error)
}

func TestGetNoHiddenCollections_Success(t *testing.T) {
	assert := assert.New(t)

	nfts := []*persist.Nft{
		{Description: "asd", OwnerUserID: tc.user1.id, CollectorsNote: "asd", OwnerAddress: tc.user1.address},
		{Description: "bbb", OwnerUserID: tc.user1.id, CollectorsNote: "bbb", OwnerAddress: tc.user1.address},
		{Description: "wowowowow", OwnerUserID: tc.user1.id, CollectorsNote: "wowowowow", OwnerAddress: tc.user1.address},
	}
	nftIDs, err := persist.NftCreateBulk(context.Background(), nfts, tc.r)

	_, err = persist.CollCreate(context.Background(), &persist.CollectionDB{
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

	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/collections/user_get?user_id=%s", tc.serverURL, tc.user1.id),
		nil)
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user2.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertValidResponse(assert, resp)

	type CollectionsResponse struct {
		Collections []*persist.Collection `json:"collections"`
		Error       string                `json:"error"`
	}

	body := CollectionsResponse{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Len(body.Collections, 2)
	assert.Empty(body.Error)
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
