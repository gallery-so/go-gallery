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

func TestGetTokenByID_Success(t *testing.T) {
	assert := setupTest(t, 2)

	// seed DB with nft
	name := "very cool nft"
	nftID, err := tc.repos.tokenRepository.Create(context.Background(), persist.Token{
		Name:           name,
		CollectorsNote: "this is a bad note",
		OwnerAddress:   tc.user1.address,
	})
	assert.Nil(err)

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nftID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	type NftGetByIDResponse struct {
		Nft   persist.Token `json:"nft"`
		Error string        `json:"error"`
	}
	body := &NftGetByIDResponse{}
	util.UnmarshallBody(body, resp.Body)
	assert.Empty(body.Error)
	assert.Equal(name, body.Nft.Name)
}

func TestGetTokenByID_NoParamError(t *testing.T) {
	assert := setupTest(t, 2)

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get", tc.serverURL))
	assert.Nil(err)
	assertErrorResponse(assert, resp)

	body := util.ErrorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.NotEmpty(body.Error)
}

func TestGetTokenByID_NotFoundError(t *testing.T) {
	assert := setupTest(t, 2)

	nonexistentNftID := "12345"

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nonexistentNftID))
	assert.Nil(err)
	assertErrorResponse(assert, resp)

	body := util.ErrorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.NotEmpty(fmt.Sprintf("token not found by ID:: %s", nonexistentNftID), body.Error)
}

func TestUpdateTokenByID_Success(t *testing.T) {
	assert := setupTest(t, 2)

	// seed DB with nft
	nftID, err := tc.repos.tokenRepository.Create(context.Background(), persist.Token{
		Name:           "very cool nft",
		CollectorsNote: "silly note",
		OwnerAddress:   tc.user1.address,
	})
	assert.Nil(err)

	resp := updateTokenRequest(assert, nftID, "new nft note", tc.user1.jwt)
	assertValidResponse(assert, resp)

	errResp := util.ErrorResponse{}
	assert.NoError(util.UnmarshallBody(&errResp, resp.Body))
	assert.Empty(errResp.Error)

	// retrieve updated nft
	resp, err = http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nftID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	// ensure nft was updated
	body := &getTokenOutput{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal("new nft note", body.Nft.CollectorsNote)
}

func TestUpdateTokenByID_UnauthedError(t *testing.T) {
	assert := setupTest(t, 2)

	// seed DB with nft
	nftID, err := tc.repos.tokenRepository.Create(context.Background(), persist.Token{
		Name:           "very cool nft",
		CollectorsNote: "this is a bad note",
		OwnerAddress:   tc.user1.address,
	})
	assert.Nil(err)

	update := updateTokenByIDInput{CollectorsNote: "new nft note thats much better", ID: nftID}
	resp := updateTokenUnauthedRequest(assert, update)
	assertErrorResponse(assert, resp)

	body := util.ErrorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Equal(middleware.ErrInvalidAuthHeader.Error(), body.Error)
}

func TestUpdateTokenByID_NoIDFieldError(t *testing.T) {
	assert := setupTest(t, 2)

	resp := updateTokenRequest(assert, "", "new nft note", tc.user1.jwt)
	assertErrorResponse(assert, resp)

	body := util.ErrorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.NotEmpty(body.Error)
}

func TestUpdateTokenByID_NotFoundError(t *testing.T) {
	assert := setupTest(t, 2)

	nftID := persist.DBID("no exist :(")

	resp := updateTokenRequest(assert, nftID, "new nft note", tc.user1.jwt)
	assertErrorResponse(assert, resp)

	body := util.ErrorResponse{}
	util.UnmarshallBody(&body, resp.Body)
	assert.NotEmpty(body.Error)
}

func TestUpdateTokenByID_UpdatingAsUserWithoutToken_CantDo(t *testing.T) {
	assert := setupTest(t, 2)

	// seed DB with nft
	nftID, err := tc.repos.tokenRepository.Create(context.Background(), persist.Token{
		Name: "very cool nft",
	})
	assert.Nil(err)

	resp := updateTokenRequest(assert, nftID, "new nft name", tc.user2.jwt)
	assertErrorResponse(assert, resp)

}

func updateTokenUnauthedRequest(assert *assert.Assertions, update updateTokenByIDInput) *http.Response {
	data, err := json.Marshal(update)
	assert.Nil(err)

	resp, err := http.Post(fmt.Sprintf("%s/nfts/update", tc.serverURL),
		"application/json",
		bytes.NewBuffer(data))
	assert.Nil(err)
	return resp
}

func updateTokenRequest(assert *assert.Assertions, id persist.DBID, collectorsNote string, jwt string) *http.Response {
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
