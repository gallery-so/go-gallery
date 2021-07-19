package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

func TestGetNftById_Success(t *testing.T) {
	assert, testServer, serverUrl, r := setup(t)
	defer teardown(testServer)

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
	assert, testServer, serverUrl, r := setup(t)
	defer teardown(testServer)

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get", serverUrl))
	assert.Nil(err)
	assertGalleryErrorResponse(assert, resp)

	body := ErrorResponse{}
	runtime.UnmarshalBody(&body, resp.Body, r)
	assert.Equal(nftIdQueryNotProvided, body.Error)
}

func TestGetNftById_NotFoundError(t *testing.T) {
	assert, testServer, serverUrl, r := setup(t)
	defer teardown(testServer)

	nonexistentNftId := "12345"

	resp, err := http.Get(fmt.Sprintf("%s/nfts/get?id=%s", serverUrl, nonexistentNftId))
	assert.Nil(err)
	assertGalleryErrorResponse(assert, resp)

	body := ErrorResponse{}
	runtime.UnmarshalBody(&body, resp.Body, r)
	assert.Equal(fmt.Sprintf("no nfts found with id: %s", nonexistentNftId), body.Error)
}

func TestHealthcheck(t *testing.T) {
	assert, testServer, serverUrl, r := setup(t)
	defer teardown(testServer)

	resp, err := http.Get(fmt.Sprintf("%s/health", serverUrl))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := HealthcheckResponse{}
	runtime.UnmarshalBody(&body, resp.Body, r)
	assert.Equal("gallery operational", body.Message)
	assert.Equal("local", body.Env)
}
