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

func TestGetNftById_Success(t *testing.T) {
	assert := assert.New(t)

	// seed DB with nft
	name := "very cool nft"
	nftId, err := persist.NftCreate(&persist.Nft{
		NameStr: name,
	}, context.Background(), r)
	assert.Nil(err)

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", serverUrl, nftId))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.Nft{}
	runtime.UnmarshalBody(&body, resp.Body, r)
	assert.Equal(name, body.NameStr)
}

func TestGetNftById_NoParamError(t *testing.T) {
	assert := assert.New(t)

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get", serverUrl))
	assert.Nil(err)
	assertGalleryErrorResponse(assert, resp)

	body := ErrorResponse{}
	runtime.UnmarshalBody(&body, resp.Body, r)
	assert.Equal(nftIdQueryNotProvided, body.Error)
}

func TestGetNftById_NotFoundError(t *testing.T) {
	assert := assert.New(t)

	nonexistentNftId := "12345"

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", serverUrl, nonexistentNftId))
	assert.Nil(err)
	assertGalleryErrorResponse(assert, resp)

	body := ErrorResponse{}
	runtime.UnmarshalBody(&body, resp.Body, r)
	assert.Equal(fmt.Sprintf("no nfts found with id: %s", nonexistentNftId), body.Error)
}

func TestUpdateNftById_Success(t *testing.T) {
	assert := assert.New(t)

	// seed DB with nft
	nftId, err := persist.NftCreate(&persist.Nft{
		NameStr: "very cool nft",
	}, context.Background(), r)
	assert.Nil(err)

	// build update request body
	type Update struct {
		Id   string `json:"id"`
		Name string `json:"name"`
	}
	update := Update{Name: "new nft name", Id: string(nftId)}
	data, err := json.Marshal(update)
	assert.Nil(err)

	resp, err := http.Post(fmt.Sprintf("%s/nfts/update?id=%s", serverUrl, nftId),
		"application/json",
		bytes.NewBuffer(data))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := persist.Nft{}
	runtime.UnmarshalBody(&body, resp.Body, r)
	assert.Equal(update.Name, body.NameStr)
}