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

func TestGetNftById_Success(t *testing.T) {
	assert := assert.New(t)

	// seed DB with nft
	name := "very cool nft"
	nftId, err := persist.NftCreate(&persist.Nft{
		NameStr: name,
	}, context.Background(), tc.r)
	assert.Nil(err)

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverUrl, nftId))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.Nft{}
	runtime.UnmarshalBody(&body, resp.Body, tc.r)
	assert.Equal(name, body.NameStr)
}

func TestGetNftById_NoParamError(t *testing.T) {
	assert := assert.New(t)

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get", tc.serverUrl))
	assert.Nil(err)
	assertGalleryErrorResponse(assert, resp)

	body := ErrorResponse{}
	runtime.UnmarshalBody(&body, resp.Body, tc.r)
	assert.Equal(copy.NftIdQueryNotProvided, body.Error)
}

func TestGetNftById_NotFoundError(t *testing.T) {
	assert := assert.New(t)

	nonexistentNftId := "12345"

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverUrl, nonexistentNftId))
	assert.Nil(err)
	assertGalleryErrorResponse(assert, resp)

	body := ErrorResponse{}
	runtime.UnmarshalBody(&body, resp.Body, tc.r)
	assert.Equal(fmt.Sprintf("no nfts found with id: %s", nonexistentNftId), body.Error)
}

func TestUpdateNftById_Success(t *testing.T) {
	assert := assert.New(t)

	// seed DB with nft
	nftId, err := persist.NftCreate(&persist.Nft{
		NameStr: "very cool nft",
		OwnerUserIdStr: tc.user1.id,
	}, context.Background(), tc.r)
	assert.Nil(err)

	// build update request body
	type Update struct {
		Id   string `json:"id"`
		Name string `json:"name"`
	}
	update := Update{Name: "new nft name", Id: string(nftId)}
	data, err := json.Marshal(update)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/nfts/update", tc.serverUrl),
		bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertValidResponse(assert, resp)

	// retrieve updated nft
	resp, err = http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverUrl, nftId))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	// ensure nft was updated
	body := persist.Nft{}
	runtime.UnmarshalBody(&body, resp.Body, tc.r)
	assert.Equal(update.Name, body.NameStr)
}

func TestUpdateNftById_UnauthedError(t *testing.T) {
	assert := assert.New(t)

	// seed DB with nft
	nftId, err := persist.NftCreate(&persist.Nft{
		NameStr: "very cool nft",
	}, context.Background(), tc.r)
	assert.Nil(err)

	// build update request body
	type Update struct {
		Id   string `json:"id"`
		Name string `json:"name"`
	}
	update := Update{Name: "new nft name", Id: string(nftId)}
	data, err := json.Marshal(update)
	assert.Nil(err)
	
	resp, err := http.Post(fmt.Sprintf("%s/nfts/update", tc.serverUrl),
		"application/json",
		bytes.NewBuffer(data))
	assert.Nil(err)
	assertGalleryErrorResponse(assert, resp)

	body := ErrorResponse{}
	runtime.UnmarshalBody(&body, resp.Body, tc.r)
	assert.Equal(copy.InvalidAuthHeader, body.Error)
}

func TestUpdateNftById_NoIdFieldError(t *testing.T) {
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
		fmt.Sprintf("%s/nfts/update", tc.serverUrl),
		bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertGalleryErrorResponse(assert, resp)

	body := ErrorResponse{}
	runtime.UnmarshalBody(&body, resp.Body, tc.r)
	assert.NotEmpty(body.Error)
}

func TestUpdateNftById_NotFoundError(t *testing.T) {
	assert := assert.New(t)

	// build update request body
	type Update struct {
		Id   string `json:"id"`
		Name string `json:"name"`
	}
	nftId := "no_existe"
	update := Update{Name: "new nft name", Id: string(nftId)}
	data, err := json.Marshal(update)
	assert.Nil(err)
	
	// send update request
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/nfts/update", tc.serverUrl),
		bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertGalleryErrorResponse(assert, resp)

	body := ErrorResponse{}
	runtime.UnmarshalBody(&body, resp.Body, tc.r)
	assert.Equal(copy.CouldNotFindDocument, body.Error)
}

func TestUpdateNftById_UpdatingAsUserWithoutToken_CantDo(t *testing.T) {
	assert := assert.New(t)

	// seed DB with nft
	nftId, err := persist.NftCreate(&persist.Nft{
		NameStr: "very cool nft",
	}, context.Background(), tc.r)
	assert.Nil(err)

	// build update request body
	type Update struct {
		Id   string `json:"id"`
		Name string `json:"name"`
	}
	update := Update{Name: "new nft name", Id: string(nftId)}
	data, err := json.Marshal(update)
	assert.Nil(err)

	// send update request WITHOUT authorization header
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/nfts/update", tc.serverUrl),
		bytes.NewBuffer(data))
	assert.Nil(err)
	client := &http.Client{}
	_, err = client.Do(req)
	assert.Nil(err)

	// retrieve updated nft
	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverUrl, nftId))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	// ensure nft was NOT updated
	body := persist.Nft{}
	runtime.UnmarshalBody(&body, resp.Body, tc.r)
	assert.NotEqual(update.Name, body.NameStr)
}

func TestUpdateNftById_UpdatingSomeoneElsesNft_CantDo(t *testing.T) {
	assert := assert.New(t)

	// seed DB with nft
	nftId, err := persist.NftCreate(&persist.Nft{
		NameStr: "very cool nft",
		OwnerUserIdStr: tc.user1.id,
	}, context.Background(), tc.r)
	assert.Nil(err)

	// build update request body
	type Update struct {
		Id   string `json:"id"`
		Name string `json:"name"`
	}
	update := Update{Name: "new nft name", Id: string(nftId)}
	data, err := json.Marshal(update)
	assert.Nil(err)

	// send update request with someone else's JWT
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/nfts/update", tc.serverUrl),
		bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user2.jwt))
	client := &http.Client{}
	_, err = client.Do(req)
	assert.Nil(err)

	// retrieve updated nft
	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", tc.serverUrl, nftId))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	// ensure nft was NOT updated
	body := persist.Nft{}
	runtime.UnmarshalBody(&body, resp.Body, tc.r)
	assert.NotEqual(update.Name, body.NameStr)
}