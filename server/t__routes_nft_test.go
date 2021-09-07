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
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/stretchr/testify/assert"
)

func TestGetNftByID_Success(t *testing.T) {
	t.Cleanup(clearDB)
	assert := assert.New(t)

	// seed DB with nft
	name := "very cool nft"
	nftID, err := persist.NftCreate(context.Background(), &persist.Nft{
		Name: name,
	}, tc.r)
	assert.Nil(err)

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nftID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.Nft{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Equal(name, body.Name)
}

func TestGetNftByID_NoParamError(t *testing.T) {
	assert := assert.New(t)

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get", tc.serverURL))
	assert.Nil(err)
	assertGalleryErrorResponse(assert, resp)

	body := errorResponse{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Equal(copy.NftIDQueryNotProvided, body.Error)
}

func TestGetNftByID_NotFoundError(t *testing.T) {
	t.Cleanup(clearDB)
	assert := assert.New(t)

	nonexistentNftID := "12345"

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nonexistentNftID))
	assert.Nil(err)
	assertGalleryErrorResponse(assert, resp)

	body := errorResponse{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Equal(fmt.Sprintf("no nfts found with id: %s", nonexistentNftID), body.Error)
}

func TestUpdateNftByID_Success(t *testing.T) {
	t.Cleanup(clearDB)
	assert := assert.New(t)

	// seed DB with nft
	nftID, err := persist.NftCreate(context.Background(), &persist.Nft{
		Name:        "very cool nft",
		OwnerUserID: tc.user1.id,
	}, tc.r)
	assert.Nil(err)

	// build update request body
	type Update struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	update := Update{Name: "new nft name", ID: string(nftID)}
	data, err := json.Marshal(update)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/nfts/update", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertValidResponse(assert, resp)

	// retrieve updated nft
	resp, err = http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nftID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	// ensure nft was updated
	body := persist.Nft{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Equal(update.Name, body.Name)
}

func TestUpdateNftByID_UnauthedError(t *testing.T) {
	t.Cleanup(clearDB)
	assert := assert.New(t)

	// seed DB with nft
	nftID, err := persist.NftCreate(context.Background(), &persist.Nft{
		Name: "very cool nft",
	}, tc.r)
	assert.Nil(err)

	// build update request body
	type Update struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	update := Update{Name: "new nft name", ID: string(nftID)}
	data, err := json.Marshal(update)
	assert.Nil(err)

	resp, err := http.Post(fmt.Sprintf("%s/nfts/update", tc.serverURL),
		"application/json",
		bytes.NewBuffer(data))
	assert.Nil(err)
	assertGalleryErrorResponse(assert, resp)

	body := errorResponse{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Equal(copy.InvalidAuthHeader, body.Error)
}

func TestUpdateNftByID_NoIDFieldError(t *testing.T) {
	t.Cleanup(clearDB)
	assert := assert.New(t)

	// build update request body
	type Update struct {
		Name string `json:"name"`
	}
	update := Update{Name: "new nft name"}
	data, err := json.Marshal(update)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/nfts/update", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertGalleryErrorResponse(assert, resp)

	body := errorResponse{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.NotEmpty(body.Error)
}

func TestUpdateNftByID_NotFoundError(t *testing.T) {
	t.Cleanup(clearDB)
	assert := assert.New(t)

	// build update request body
	type Update struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	nftID := "no_existe"
	update := Update{Name: "new nft name", ID: string(nftID)}
	data, err := json.Marshal(update)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/nfts/update", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertGalleryErrorResponse(assert, resp)

	body := errorResponse{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Equal(copy.CouldNotFindDocument, body.Error)
}

func TestUpdateNftByID_UpdatingAsUserWithoutToken_CantDo(t *testing.T) {
	t.Cleanup(clearDB)
	assert := assert.New(t)

	// seed DB with nft
	nftID, err := persist.NftCreate(context.Background(), &persist.Nft{
		Name: "very cool nft",
	}, tc.r)
	assert.Nil(err)

	// build update request body
	type Update struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	update := Update{Name: "new nft name", ID: string(nftID)}
	data, err := json.Marshal(update)
	assert.Nil(err)

	// send update request WITHOUT authorization header
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/nfts/update", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)
	client := &http.Client{}
	_, err = client.Do(req)
	assert.Nil(err)

	// retrieve updated nft
	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nftID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	// ensure nft was NOT updated
	body := persist.Nft{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.NotEqual(update.Name, body.Name)
}

func TestUpdateNftByID_UpdatingSomeoneElsesNft_CantDo(t *testing.T) {
	t.Cleanup(clearDB)
	assert := assert.New(t)

	// seed DB with nft
	nftID, err := persist.NftCreate(context.Background(), &persist.Nft{
		Name:        "very cool nft",
		OwnerUserID: tc.user1.id,
	}, tc.r)
	assert.Nil(err)

	// build update request body
	type Update struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	update := Update{Name: "new nft name", ID: string(nftID)}
	data, err := json.Marshal(update)
	assert.Nil(err)

	// send update request with someone else's JWT
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/nfts/update", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user2.jwt))
	client := &http.Client{}
	_, err = client.Do(req)
	assert.Nil(err)

	// retrieve updated nft
	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverURL, nftID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	// ensure nft was NOT updated
	body := persist.Nft{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.NotEqual(update.Name, body.Name)
}
