package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/assert"
)

func TestSync_Success(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	user := &persist.User{
		UserName:           "Benny",
		UserNameIdempotent: "benny",
		Addresses:          []string{"0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5"},
	}
	id, err := persist.UserCreate(context.Background(), user, tc.r)
	assert.NoError(err)
	jwt, err := jwtGeneratePipeline(context.Background(), id, tc.r)
	assert.NoError(err)

	resp := syncRequest(assert, user.Addresses[0], jwt)
	assertValidResponse(assert, resp)

	type SyncResp struct {
		Nfts  []*persist.Token `json:"nfts"`
		Error string           `json:"error"`
	}
	output := &SyncResp{}
	err = util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.Empty(output.Error)
	assert.Greater(len(output.Nfts), 0)
}

func syncRequest(assert *assert.Assertions, walletAddress, jwt string) *http.Response {
	// send update request
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/nfts/sync?address=%s&skip_db=true", tc.serverURL, walletAddress),
		nil)
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	return resp
}
