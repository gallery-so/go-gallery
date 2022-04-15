package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/assert"
)

func TestTokenRoutes(t *testing.T) {

	setupDoubles(t)

	t.Run("get token by ID", func(t *testing.T) {
		assert := setupTest(t, 2)

		// seed DB with nft
		name := "very cool nft"
		nftID, err := tc.repos.TokenRepository.Create(context.Background(), persist.TokenGallery{
			Name:           persist.NullString(name),
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
		assert.Equal(name, body.Nft.Name.String())
	})

	t.Run("get token with no ID errors", func(t *testing.T) {
		assert := setupTest(t, 2)

		resp, err := http.Get(fmt.Sprintf("%s/nfts/get", tc.serverURL))
		assert.Nil(err)
		assertErrorResponse(assert, resp)

		body := util.ErrorResponse{}
		util.UnmarshallBody(&body, resp.Body)
		assert.NotEmpty(body.Error)
	})

	t.Run("get non-existent token errors", func(t *testing.T) {
		assert := setupTest(t, 2)

		nonexistentNftID := "12345"

		resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nonexistentNftID))
		assert.Nil(err)
		assertErrorResponse(assert, resp)

		body := util.ErrorResponse{}
		util.UnmarshallBody(&body, resp.Body)
		assert.NotEmpty(fmt.Sprintf("token not found by ID:: %s", nonexistentNftID), body.Error)
	})

	t.Run("referesh metadata", func(t *testing.T) {
		assert := setupTest(t, 2)

		// seed DB with nft
		name := "very cool nft"
		token := persist.TokenGallery{
			Name:            persist.NullString(name),
			CollectorsNote:  "this is a bad note",
			OwnerAddress:    tc.user1.address,
			ContractAddress: "0xb74bf94049d2c01f8805b8b15db0909168cabf46",
			TokenID:         "2ad",
			TokenURI:        "https://bad-uri.com",
		}
		_, err := tc.repos.TokenRepository.Create(context.Background(), token)
		assert.Nil(err)

		resp, err := http.Get(fmt.Sprintf("%s/nfts/metadata/refresh?token_id=%s&contract_address=%s", tc.serverURL, token.TokenID, token.ContractAddress))
		assert.Nil(err)
		assertValidJSONResponse(assert, resp)

		type RefreshResponse struct {
			Nft   persist.Token `json:"token"`
			Error string        `json:"error"`
		}
		body := &RefreshResponse{}
		util.UnmarshallBody(body, resp.Body)
		assert.Empty(body.Error)
		assert.Equal(name, body.Nft.Name.String())
		assert.NotEqual(token.TokenURI, body.Nft.TokenURI)
	})

	t.Run("update token by ID", func(t *testing.T) {
		assert := setupTest(t, 2)

		// seed DB with nft
		nftID, err := tc.repos.TokenRepository.Create(context.Background(), persist.TokenGallery{
			Name:           "very cool nft",
			CollectorsNote: "silly note",
			OwnerAddress:   tc.user1.address,
		})
		assert.Nil(err)

		resp := updateTokenRequest(assert, nftID, "new nft note", tc.user1)
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
		assert.Equal("new nft note", body.Nft.CollectorsNote.String())
	})

	t.Run("update token by ID fails when unauthenticated", func(t *testing.T) {
		assert := setupTest(t, 2)

		// seed DB with nft
		nftID, err := tc.repos.TokenRepository.Create(context.Background(), persist.TokenGallery{
			Name:           "very cool nft",
			CollectorsNote: "this is a bad note",
			OwnerAddress:   tc.user1.address,
		})
		assert.Nil(err)

		update := updateTokenByIDInput{CollectorsNote: "new nft note thats much better", ID: nftID}
		resp := updateTokenUnauthedRequest(assert, update)
		assertErrorResponse(assert, resp)

		assert.Equal(resp.StatusCode, http.StatusUnauthorized)
	})

	t.Run("update token by ID fails when no ID provided", func(t *testing.T) {
		assert := setupTest(t, 2)

		resp := updateTokenRequest(assert, "", "new nft note", tc.user1)
		assertErrorResponse(assert, resp)

		body := util.ErrorResponse{}
		util.UnmarshallBody(&body, resp.Body)
		assert.NotEmpty(body.Error)
	})

	t.Run("update token fails when not found", func(t *testing.T) {
		assert := setupTest(t, 2)

		nftID := persist.DBID("no exist :(")

		resp := updateTokenRequest(assert, nftID, "new nft note", tc.user1)
		assertErrorResponse(assert, resp)

		body := util.ErrorResponse{}
		util.UnmarshallBody(&body, resp.Body)
		assert.NotEmpty(body.Error)
	})

	t.Run("update token fails when user does not own NFT", func(t *testing.T) {
		assert := setupTest(t, 2)

		// seed DB with nft
		nftID, err := tc.repos.TokenRepository.Create(context.Background(), persist.TokenGallery{
			Name: "very cool nft",
		})
		assert.Nil(err)

		resp := updateTokenRequest(assert, nftID, "new nft name", tc.user2)
		assertErrorResponse(assert, resp)
	})
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

func updateTokenRequest(assert *assert.Assertions, id persist.DBID, collectorsNote string, tu *TestUser) *http.Response {
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

	resp, err := tu.client.Do(req)
	assert.Nil(err)
	return resp
}
