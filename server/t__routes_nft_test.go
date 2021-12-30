package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/assert"
)

func TestGetNftByID_Success(t *testing.T) {
	assert := setupTest(t, 1)

	// seed DB with nft
	name := "very cool nft"
	nftID, err := tc.repos.nftRepository.Create(context.Background(), persist.NFTDB{
		Name:         persist.NullString(name),
		OwnerAddress: tc.user1.address,
	})
	assert.Nil(err)

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nftID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	type NftGetByIDResponse struct {
		Nft   persist.NFT `json:"nft"`
		Error string      `json:"error"`
	}
	body := &NftGetByIDResponse{}
	util.UnmarshallBody(body, resp.Body)
	assert.Empty(body.Error)
	assert.Equal(name, body.Nft.Name)
}

func TestGetNftByID_NoParamError(t *testing.T) {
	assert := setupTest(t, 1)

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get", tc.serverURL))
	assert.Nil(err)
	assertErrorResponse(assert, resp)

	body := util.ErrorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.NotEmpty(body.Error)
}

func TestGetNftByID_NotFoundError(t *testing.T) {
	assert := setupTest(t, 1)

	nonexistentNftID := "12345"

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nonexistentNftID))
	assert.Nil(err)
	assertErrorResponse(assert, resp)

	body := util.ErrorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(fmt.Sprintf("could not find NFT with ID: %s", nonexistentNftID), body.Error)
}

func TestUpdateNftByID_Success(t *testing.T) {
	assert := setupTest(t, 1)

	// seed DB with nft
	nftID, err := tc.repos.nftRepository.Create(context.Background(), persist.NFTDB{
		Name:           "very cool nft",
		CollectorsNote: "silly note",
		OwnerAddress:   tc.user1.address,
	})
	assert.Nil(err)

	update := updateNftByIDInput{CollectorsNote: "new nft note", ID: nftID}
	resp := updateNFTRequest(assert, update, tc.user1.jwt)
	assertValidResponse(assert, resp)
	errResp := util.ErrorResponse{}
	json.NewDecoder(resp.Body).Decode(&errResp)
	assert.Empty(errResp.Error)

	// retrieve updated nft
	resp, err = http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nftID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	// ensure nft was updated
	type NftGetByIDResponse struct {
		Nft   persist.NFT `json:"nft"`
		Error string      `json:"error"`
	}
	body := &NftGetByIDResponse{}
	util.UnmarshallBody(body, resp.Body)
	assert.Empty(body.Error)
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(update.CollectorsNote, body.Nft.CollectorsNote)
}

func TestUpdateNftByID_UnauthedError(t *testing.T) {
	assert := setupTest(t, 1)

	// seed DB with nft
	nftID, err := tc.repos.nftRepository.Create(context.Background(), persist.NFTDB{
		Name:           "very cool nft",
		CollectorsNote: "this is a bad note",
		OwnerAddress:   tc.user1.address,
	})
	assert.Nil(err)

	update := updateNftByIDInput{CollectorsNote: "new nft note thats much better", ID: nftID}
	resp := updateNFTUnauthedRequest(assert, update)
	assertErrorResponse(assert, resp)

	body := util.ErrorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(middleware.ErrInvalidAuthHeader.Error(), body.Error)
}

func TestUpdateNftByID_NoIDFieldError(t *testing.T) {
	assert := setupTest(t, 1)

	update := updateNftByIDInput{CollectorsNote: "new nft note"}
	resp := updateNFTRequest(assert, update, tc.user1.jwt)
	assertErrorResponse(assert, resp)

	body := util.ErrorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.NotEmpty(body.Error)
}

func TestUpdateNftByID_NotFoundError(t *testing.T) {
	assert := setupTest(t, 1)

	nftID := persist.DBID("no exist :(")
	update := updateNftByIDInput{CollectorsNote: "new nft note", ID: nftID}
	resp := updateNFTRequest(assert, update, tc.user1.jwt)
	assertErrorResponse(assert, resp)

	body := util.ErrorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.NotEmpty(body.Error)
}

func TestUpdateNftByID_UpdatingAsUserWithoutToken_CantDo(t *testing.T) {
	assert := setupTest(t, 1)

	// seed DB with nft
	nftID, err := tc.repos.nftRepository.Create(context.Background(), persist.NFTDB{
		Name: "very cool nft",
	})
	assert.Nil(err)

	update := updateNftByIDInput{CollectorsNote: "new nft name", ID: nftID}
	resp := updateNFTRequest(assert, update, tc.user2.jwt)
	assertErrorResponse(assert, resp)

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

func updateNFTRequest(assert *assert.Assertions, update updateNftByIDInput, jwt string) *http.Response {
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
