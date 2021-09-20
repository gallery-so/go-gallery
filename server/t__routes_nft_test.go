package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/mikeydub/go-gallery/copy"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/assert"
)

func TestGetNftByID_Success(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	// seed DB with nft
	name := "very cool nft"
	nftID, err := persist.TokenCreate(context.Background(), &persist.Token{
		CollectorsNote: name,
	}, tc.r)
	assert.Nil(err)

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nftID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.Token{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(name, body.CollectorsNote)
}

func TestGetNftByID_NoParamError(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get", tc.serverURL))
	assert.Nil(err)
	assertGalleryErrorResponse(assert, resp)

	body := util.ErrorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(copy.NftIDQueryNotProvided, body.Error)
}

func TestGetNftByID_NotFoundError(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	nonexistentNftID := "12345"

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nonexistentNftID))
	assert.Nil(err)
	assertGalleryErrorResponse(assert, resp)

	body := util.ErrorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(fmt.Sprintf("no nfts found with id: %s", nonexistentNftID), body.Error)
}

func TestUpdateNftByID_Success(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	// seed DB with nft
	nftID, err := persist.TokenCreate(context.Background(), &persist.Token{
		CollectorsNote: "silly note",
		OwnerUserID:    tc.user1.id,
	}, tc.r)
	assert.Nil(err)

	resp := updateNFTRequest(assert, nftID, "new nft note", tc.user1.jwt)
	assertValidResponse(assert, resp)

	// retrieve updated nft
	resp, err = http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nftID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	// ensure nft was updated
	body := persist.Token{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal("new nft note", body.CollectorsNote)
}

func TestUpdateNftByID_UnauthedError(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	// seed DB with nft
	nftID, err := persist.TokenCreate(context.Background(), &persist.Token{
		CollectorsNote: "this is a bad note",
		OwnerUserID:    tc.user1.id,
	}, tc.r)
	assert.Nil(err)

	update := updateNftByIDInput{CollectorsNote: "new nft note thats much better", ID: nftID}
	resp := updateNFTUnauthedRequest(assert, update)
	assertGalleryErrorResponse(assert, resp)

	body := util.ErrorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(copy.InvalidAuthHeader, body.Error)
}

func TestUpdateNftByID_NoIDFieldError(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	resp := updateNFTRequest(assert, "", "new nft note", tc.user1.jwt)
	assertGalleryErrorResponse(assert, resp)

	body := util.ErrorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.NotEmpty(body.Error)
}

func TestUpdateNftByID_NotFoundError(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	nftID := persist.DBID("no exist :(")

	resp := updateNFTRequest(assert, nftID, "new nft note", tc.user1.jwt)
	assertGalleryErrorResponse(assert, resp)

	body := util.ErrorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(copy.CouldNotFindDocument, body.Error)
}

func TestUpdateNftByID_UpdatingAsUserWithoutToken_CantDo(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	// seed DB with nft
	nftID, err := persist.TokenCreate(context.Background(), &persist.Token{
		OwnerUserID: tc.user1.id,
	}, tc.r)
	assert.Nil(err)

	resp := updateNFTRequest(assert, nftID, "new nft name", tc.user2.jwt)
	assertGalleryErrorResponse(assert, resp)

}

func updateNFTUnauthedRequest(assert *assert.Assertions, update updateNftByIDInput) *http.Response {
	data, err := json.Marshal(update)
	assert.Nil(err)

	resp, err := http.Post(fmt.Sprintf("%s/nfts/update", tc.serverURL),
		"application/json",
		bytes.NewBuffer(data))
	assert.Nil(err)
	return resp
}

func updateNFTRequest(assert *assert.Assertions, id persist.DBID, collectorsNote string, jwt string) *http.Response {
	update := map[string]interface{}{
		"id":              id,
		"collectors_note": collectorsNote,
	}
	data, err := json.Marshal(update)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/nfts/update", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	return resp
}
