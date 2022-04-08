package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestCollectionRoutes(t *testing.T) {

	setupDoubles(t)

	t.Run("update collection name by ID", func(t *testing.T) {
		assert := setupTest(t, 1)

		nft := persist.NFT{
			Description:    "asd",
			OwnerAddress:   tc.user1.address,
			CollectorsNote: "asd",
		}
		nftID, err := tc.repos.NftRepository.Create(context.Background(), nft)
		assert.Nil(err)
		// seed DB with collection
		collID, err := tc.repos.CollectionRepository.Create(context.Background(), persist.CollectionDB{
			Name:        "very cool collection",
			OwnerUserID: tc.user1.id,
			NFTs:        []persist.DBID{nftID},
		})
		assert.Nil(err)

		// build update request body
		update := collectionUpdateInfoByIDInput{Name: "new coll name", ID: collID}
		resp := updateCollectionInfoRequest(assert, update, tc.user1)

		errResp := &util.ErrorResponse{}
		err = util.UnmarshallBody(errResp, resp.Body)
		assert.Nil(err)
		assert.Empty(errResp.Error)

		assertValidResponse(assert, resp)

		// retrieve updated nft
		resp, err = http.Get(fmt.Sprintf("%s/collections/get?id=%s", tc.serverURL, collID))
		assert.Nil(err)
		assertValidJSONResponse(assert, resp)

		type CollectionGetResponse struct {
			Collection persist.Collection `json:"collection"`
			Error      string             `json:"error"`
		}
		// ensure nft was updated
		body := CollectionGetResponse{}
		util.UnmarshallBody(&body, resp.Body)
		assert.Empty(body.Error)
		assert.Equal(update.Name, body.Collection.Name.String())
		assert.NotEmpty(body.Collection.ID)
	})

	t.Run("update collection, collector's note is too long", func(t *testing.T) {
		assert := setupTest(t, 1)

		nft := persist.NFT{
			Description:    "asd",
			OwnerAddress:   tc.user1.address,
			CollectorsNote: "asd",
		}
		nftID, err := tc.repos.NftRepository.Create(context.Background(), nft)
		assert.Nil(err)
		// seed DB with collection
		collID, err := tc.repos.CollectionRepository.Create(context.Background(), persist.CollectionDB{
			Name:        "very cool collection",
			OwnerUserID: tc.user1.id,
			NFTs:        []persist.DBID{nftID},
		})
		assert.Nil(err)

		// build update request body
		update := collectionUpdateInfoByIDInput{Name: "new coll name", ID: collID, CollectorsNote: util.RandStringBytes(601)}
		resp := updateCollectionInfoRequest(assert, update, tc.user1)

		errResp := &util.ErrorResponse{}
		err = util.UnmarshallBody(errResp, resp.Body)
		assert.Nil(err)
		assert.NotEmpty(errResp.Error)

		assertErrorResponse(assert, resp)
	})

	t.Run("create collection", func(t *testing.T) {
		assert := setupTest(t, 1)

		nfts := []persist.NFT{
			{Description: "asd", CollectorsNote: "asd", OwnerAddress: tc.user1.address, OpenseaID: 0},
			{Description: "bbb", CollectorsNote: "bbb", OwnerAddress: tc.user1.address, OpenseaID: 1},
			{Description: "wowowowow", CollectorsNote: "wowowowow", OwnerAddress: tc.user1.address, OpenseaID: 2},
		}
		nftIDs, err := tc.repos.NftRepository.CreateBulk(context.Background(), nfts)
		assert.Nil(err)
		gid, err := tc.repos.GalleryRepository.Create(context.Background(), persist.GalleryDB{OwnerUserID: tc.user1.id})
		assert.Nil(err)

		input := collectionCreateInput{GalleryID: gid, Nfts: nftIDs}
		resp := createCollectionRequest(assert, input, tc.user1)
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
			Collection persist.Collection `json:"collection"`
			Error      string             `json:"error"`
		}
		// ensure nft was updated
		body := CollectionGetResponse{}
		util.UnmarshallBody(&body, resp.Body)
		assert.Len(body.Collection.NFTs, 3)
		assert.Empty(body.Error)

		gallery, err := tc.repos.GalleryRepository.GetByID(context.Background(), gid)
		assert.Nil(err)
		assert.Len(gallery.Collections, 1)
	})

	t.Run("get unassigned collection", func(t *testing.T) {
		assert := setupTest(t, 1)

		nfts := []persist.NFT{
			{Description: "asd", CollectorsNote: "asd", OwnerAddress: tc.user1.address, OpenseaID: 0},
			{Description: "bbb", CollectorsNote: "bbb", OwnerAddress: tc.user1.address, OpenseaID: 1},
			{Description: "wowowowow", CollectorsNote: "wowowowow", OwnerAddress: tc.user1.address, OpenseaID: 2},
		}
		nftIDs, err := tc.repos.NftRepository.CreateBulk(context.Background(), nfts)
		// seed DB with collection
		_, err = tc.repos.CollectionRepository.Create(context.Background(), persist.CollectionDB{
			Name:        "very cool collection",
			OwnerUserID: tc.user1.id,
			NFTs:        nftIDs[:2],
		})
		assert.Nil(err)

		resp := getUnassignedNFTsRequest(assert, tc.user1.id, tc.user1)
		assertValidResponse(assert, resp)

		type NftsResponse struct {
			Nfts  []*persist.NFT `json:"nfts"`
			Error string         `json:"error"`
		}
		// ensure nft was updated
		body := NftsResponse{}
		util.UnmarshallBody(&body, resp.Body)
		assert.Len(body.Nfts, 1)
		assert.Empty(body.Error)
	})

	t.Run("delete collection", func(t *testing.T) {
		assert := setupTest(t, 1)

		collID := createCollectionInDbForUserID(assert, "COLLECTION NAME", tc.user1.id)
		verifyCollectionExistsInDbForID(assert, collID)

		resp := sendDeleteRequest(assert, collectionDeleteInput{ID: collID}, tc.user1)

		assertValidResponse(assert, resp)

		// Assert that the collection was deleted
		_, err := tc.repos.CollectionRepository.GetByID(context.Background(), collID, false)
		assert.NotNil(err)
	})

	t.Run("cannot delete collection if unauthenticated", func(t *testing.T) {
		assert := setupTest(t, 1)

		collID := createCollectionInDbForUserID(assert, "COLLECTION NAME", tc.user1.id)
		verifyCollectionExistsInDbForID(assert, collID)

		resp := sendDeleteRequest(assert, collectionDeleteInput{ID: collID}, nil)

		assert.Equal(401, resp.StatusCode)
	})

	t.Run("delete collection fails if not user's collection", func(t *testing.T) {
		assert := setupTest(t, 1)

		collID := createCollectionInDbForUserID(assert, "COLLECTION NAME", tc.user1.id)
		verifyCollectionExistsInDbForID(assert, collID)

		resp := sendDeleteRequest(assert, collectionDeleteInput{ID: collID}, tc.user2)
		assert.Equal(500, resp.StatusCode)
	})

	t.Run("get hidden collections", func(t *testing.T) {
		assert := setupTest(t, 1)

		nfts := []persist.NFT{
			{Description: "asd", CollectorsNote: "asd", OwnerAddress: tc.user1.address, OpenseaID: 0},
			{Description: "bbb", CollectorsNote: "bbb", OwnerAddress: tc.user1.address, OpenseaID: 1},
			{Description: "wowowowow", CollectorsNote: "wowowowow", OwnerAddress: tc.user1.address, OpenseaID: 2},
		}
		nftIDs, err := tc.repos.NftRepository.CreateBulk(context.Background(), nfts)
		assert.Nil(err)

		_, err = tc.repos.CollectionRepository.Create(context.Background(), persist.CollectionDB{
			Name:        "very cool collection",
			OwnerUserID: tc.user1.id,
			NFTs:        nftIDs,
			Hidden:      true,
		})
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
	})

	t.Run("get no hidden collections", func(t *testing.T) {
		assert := setupTest(t, 1)

		nfts := []persist.NFT{
			{Description: "asd", CollectorsNote: "asd", OwnerAddress: tc.user1.address, OpenseaID: 0},
			{Description: "bbb", CollectorsNote: "bbb", OwnerAddress: tc.user1.address, OpenseaID: 1},
			{Description: "wowowowow", CollectorsNote: "wowowowow", OwnerAddress: tc.user1.address, OpenseaID: 2},
		}
		nftIDs, err := tc.repos.NftRepository.CreateBulk(context.Background(), nfts)
		assert.Nil(err)

		_, err = tc.repos.CollectionRepository.Create(context.Background(), persist.CollectionDB{
			Name:        "very cool collection",
			OwnerUserID: tc.user1.id,
			NFTs:        nftIDs[0:1],
			Hidden:      false,
		})
		_, err = tc.repos.CollectionRepository.Create(context.Background(), persist.CollectionDB{
			Name:        "very cool collection",
			OwnerUserID: tc.user1.id,
			NFTs:        nftIDs[1:],
			Hidden:      true,
		})
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
	})

	t.Run("create collections with used NFTs", func(t *testing.T) {
		assert := setupTest(t, 1)

		nfts := []persist.NFT{
			{Description: "asd", CollectorsNote: "asd", OwnerAddress: tc.user1.address, OpenseaID: 0},
			{Description: "bbb", CollectorsNote: "bbb", OwnerAddress: tc.user1.address, OpenseaID: 1},
			{Description: "wowowowow", CollectorsNote: "wowowowow", OwnerAddress: tc.user1.address, OpenseaID: 2},
		}
		nftIDs, err := tc.repos.NftRepository.CreateBulk(context.Background(), nfts)
		assert.Nil(err)

		preCollID, err := tc.repos.CollectionRepository.Create(context.Background(), persist.CollectionDB{Name: "test", NFTs: nftIDs, OwnerUserID: tc.user1.id})
		gid, err := tc.repos.GalleryRepository.Create(context.Background(), persist.GalleryDB{Collections: []persist.DBID{preCollID}, OwnerUserID: tc.user1.id})

		input := collectionCreateInput{GalleryID: gid, Nfts: nftIDs[0:2]}
		resp := createCollectionRequest(assert, input, tc.user1)
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
		assert.Len(body.Collection.NFTs, 3)
		assert.Empty(body.Error)

	})

	t.Run("update NFT order in collection", func(t *testing.T) {
		assert := setupTest(t, 1)

		nfts := []persist.NFT{
			{Description: "asd", CollectorsNote: "asd", OwnerAddress: tc.user1.address, OpenseaID: 0},
			{Description: "bbb", CollectorsNote: "bbb", OwnerAddress: tc.user1.address, OpenseaID: 1},
			{Description: "wowowowow", CollectorsNote: "wowowowow", OwnerAddress: tc.user1.address, OpenseaID: 2},
		}
		nftIDs, err := tc.repos.NftRepository.CreateBulk(context.Background(), nfts)
		assert.Nil(err)

		collID, err := tc.repos.CollectionRepository.Create(context.Background(), persist.CollectionDB{
			Name:        "very cool collection",
			OwnerUserID: tc.user1.id,
			NFTs:        nftIDs,
		})
		assert.Nil(err)

		temp := nftIDs[1]
		nftIDs[1] = nftIDs[2]
		nftIDs[2] = temp

		update := collectionUpdateNftsByIDInput{ID: collID, Nfts: nftIDs}
		resp := updateCollectionNftsRequest(assert, update, tc.user1)
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
		logrus.Infof("nfts body: %v", body.Collection.NFTs)
		assert.Equal(update.Nfts[1], body.Collection.NFTs[1].ID)
	})
}

func verifyCollectionExistsInDbForID(assert *assert.Assertions, collID persist.DBID) {
	coll, err := tc.repos.CollectionRepository.GetByID(context.Background(), collID, true)
	assert.Nil(err)
	assert.Equal(coll.ID, collID)
}

func sendDeleteRequest(assert *assert.Assertions, requestBody interface{}, authenticatedUser *TestUser) *http.Response {
	data, err := json.Marshal(requestBody)
	assert.Nil(err)

	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/collections/delete", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)

	if authenticatedUser != nil {
		req.AddCookie(&http.Cookie{Name: auth.JWTCookieKey, Value: authenticatedUser.jwt, Expires: time.Now().Add(time.Duration(viper.GetInt("JWT_TTL")) * time.Second), Secure: false, Path: "/", HttpOnly: true})
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)

	return resp
}

func getUnassignedNFTsRequest(assert *assert.Assertions, userID persist.DBID, tu *TestUser) *http.Response {
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/nfts/unassigned/get", tc.serverURL),
		nil)
	assert.Nil(err)

	resp, err := tu.client.Do(req)
	assert.Nil(err)
	return resp
}

func sendCollUserGetRequest(assert *assert.Assertions, forUserID string, tu *TestUser) *http.Response {

	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/collections/user_get?user_id=%s", tc.serverURL, forUserID),
		nil)
	assert.Nil(err)

	resp, err := tu.client.Do(req)
	assert.Nil(err)
	assertValidResponse(assert, resp)

	return resp
}

func createCollectionRequest(assert *assert.Assertions, input collectionCreateInput, tu *TestUser) *http.Response {
	data, err := json.Marshal(input)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/collections/create", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)
	resp, err := tu.client.Do(req)
	assert.Nil(err)
	return resp
}

func updateCollectionInfoRequest(assert *assert.Assertions, input collectionUpdateInfoByIDInput, tu *TestUser) *http.Response {
	data, err := json.Marshal(input)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/collections/update/info", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)

	resp, err := tu.client.Do(req)
	assert.Nil(err)
	return resp
}
func updateCollectionNftsRequest(assert *assert.Assertions, input collectionUpdateNftsByIDInput, tu *TestUser) *http.Response {
	data, err := json.Marshal(input)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/collections/update/nfts", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)
	resp, err := tu.client.Do(req)
	assert.Nil(err)
	return resp
}

func createCollectionInDbForUserID(assert *assert.Assertions, collectionName string, userID persist.DBID) persist.DBID {

	nfts := []persist.NFT{
		{Description: "asd", CollectorsNote: "asd", OwnerAddress: tc.user1.address, OpenseaID: persist.NullInt64(rand.Intn(10000000))},
		{Description: "bbb", CollectorsNote: "bbb", OwnerAddress: tc.user1.address, OpenseaID: persist.NullInt64(rand.Intn(10000000))},
		{Description: "wowowowow", CollectorsNote: "wowowowow", OwnerAddress: tc.user1.address, OpenseaID: persist.NullInt64(rand.Intn(10000000))},
	}
	nftIDs, err := tc.repos.NftRepository.CreateBulk(context.Background(), nfts)

	assert.Nil(err)
	collID, err := tc.repos.CollectionRepository.Create(context.Background(), persist.CollectionDB{
		Name:        persist.NullString(collectionName),
		OwnerUserID: userID,
		NFTs:        nftIDs,
	})
	assert.Nil(err)

	return collID
}
