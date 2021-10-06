package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/mikeydub/go-gallery/copy"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/assert"
)

func TestGetNftByID_Success(t *testing.T) {
	assert := setupTest(t)

	// seed DB with nft
	name := "very cool nft"
	nftID, err := tc.repos.nftRepository.Create(context.Background(), &persist.NftDB{
		Name:         name,
		OwnerAddress: strings.ToLower(tc.user1.address),
	})
	assert.Nil(err)

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nftID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	type NftGetByIDResponse struct {
		Nft   persist.Nft `json:"nft"`
		Error string      `json:"error"`
	}
	body := &NftGetByIDResponse{}
	util.UnmarshallBody(body, resp.Body)
	assert.Empty(body.Error)
	assert.Equal(name, body.Nft.Name)
}

func TestGetNftByID_NoParamError(t *testing.T) {
	assert := setupTest(t)

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get", tc.serverURL))
	assert.Nil(err)
	assertErrorResponse(assert, resp)

	body := errorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(copy.NftIDQueryNotProvided, body.Error)
}

func TestGetNftByID_NotFoundError(t *testing.T) {
	assert := setupTest(t)

	nonexistentNftID := "12345"

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nonexistentNftID))
	assert.Nil(err)
	assertErrorResponse(assert, resp)

	body := errorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(fmt.Sprintf("no nfts found with id: %s", nonexistentNftID), body.Error)
}

func TestUpdateNftByID_Success(t *testing.T) {
	assert := setupTest(t)

	// seed DB with nft
	nftID, err := tc.repos.nftRepository.Create(context.Background(), &persist.NftDB{
		Name:           "very cool nft",
		CollectorsNote: "silly note",
		OwnerAddress:   strings.ToLower(tc.user1.address),
	})
	assert.Nil(err)

	update := updateNftByIDInput{CollectorsNote: "new nft note", ID: nftID}
	resp := updateNFTRequest(assert, update, tc.user1.jwt)
	assertValidResponse(assert, resp)

	// retrieve updated nft
	resp, err = http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nftID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	// ensure nft was updated
	type NftGetByIDResponse struct {
		Nft   persist.Nft `json:"nft"`
		Error string      `json:"error"`
	}
	body := &NftGetByIDResponse{}
	util.UnmarshallBody(body, resp.Body)
	assert.Empty(body.Error)
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(update.CollectorsNote, body.Nft.CollectorsNote)
}

func TestUpdateNftByID_UnauthedError(t *testing.T) {
	assert := setupTest(t)

	// seed DB with nft
	nftID, err := tc.repos.nftRepository.Create(context.Background(), &persist.NftDB{
		Name:           "very cool nft",
		CollectorsNote: "this is a bad note",
		OwnerAddress:   strings.ToLower(tc.user1.address),
	})
	assert.Nil(err)

	update := updateNftByIDInput{CollectorsNote: "new nft note thats much better", ID: nftID}
	resp := updateNFTUnauthedRequest(assert, update)
	assertErrorResponse(assert, resp)

	body := errorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(copy.InvalidAuthHeader, body.Error)
}

func TestUpdateNftByID_NoIDFieldError(t *testing.T) {
	assert := setupTest(t)

	update := updateNftByIDInput{CollectorsNote: "new nft note"}
	resp := updateNFTRequest(assert, update, tc.user1.jwt)
	assertErrorResponse(assert, resp)

	body := errorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.NotEmpty(body.Error)
}

func TestUpdateNftByID_NotFoundError(t *testing.T) {
	assert := setupTest(t)

	nftID := persist.DBID("no exist :(")
	update := updateNftByIDInput{CollectorsNote: "new nft note", ID: nftID}
	resp := updateNFTRequest(assert, update, tc.user1.jwt)
	assertErrorResponse(assert, resp)

	body := errorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(copy.CouldNotFindDocument, body.Error)
}

func TestUpdateNftByID_UpdatingAsUserWithoutToken_CantDo(t *testing.T) {
	assert := setupTest(t)

	// seed DB with nft
	nftID, err := tc.repos.nftRepository.Create(context.Background(), &persist.NftDB{
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
