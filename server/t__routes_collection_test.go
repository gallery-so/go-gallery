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

func TestUpdateCollectionNameByID_Success(t *testing.T) {
	assert := assert.New(t)

	// seed DB with collection
	collID, err := persist.CollCreate(context.Background(), &persist.CollectionDB{
		Name:        "very cool collection",
		OwnerUserID: tc.user1.id,
	}, tc.r)
	assert.Nil(err)

	// build update request body
	type Update struct {
		ID             persist.DBID `json:"id"`
		Name           string       `json:"name"`
		CollectorsNote string       `json:"collectors_note"`
	}
	update := Update{Name: "new coll name", ID: collID}
	data, err := json.Marshal(update)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/collections/update/info", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertValidResponse(assert, resp)

	// retrieve updated nft
	resp, err = http.Get(fmt.Sprintf("%s/collections/get?id=%s", tc.serverURL, collID))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	type CollectionGetResponse struct {
		Collections []*persist.Collection `json:"collections"`
		Error       string                `json:"error"`
	}
	// ensure nft was updated
	body := CollectionGetResponse{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Len(body.Collections, 1)
	assert.Empty(body.Error)
	assert.Equal(update.Name, body.Collections[0].Name)
}

func TestGetUnassignedCollection_Success(t *testing.T) {
	assert := assert.New(t)

	nfts := []*persist.Nft{
		{Description: "asd", OwnerUserID: tc.user1.id, CollectorsNote: "asd", OwnerAddress: tc.user1.address},
		{Description: "bbb", OwnerUserID: tc.user1.id, CollectorsNote: "bbb", OwnerAddress: tc.user1.address},
		{Description: "wowowowow", OwnerUserID: tc.user1.id, CollectorsNote: "wowowowow", OwnerAddress: tc.user1.address},
	}
	nftIDs, err := persist.NftCreateBulk(context.Background(), nfts, tc.r)
	// seed DB with collection
	_, err = persist.CollCreate(context.Background(), &persist.CollectionDB{
		Name:        "very cool collection",
		OwnerUserID: tc.user1.id,
		Nfts:        nftIDs[:2],
	}, tc.r)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/nfts/get_unassigned?user_id=%s&skip_cache=false", tc.serverURL, tc.user1.id),
		nil)
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertValidResponse(assert, resp)

	type NftsResponse struct {
		Nfts  []*persist.Nft `json:"nfts"`
		Error string         `json:"error"`
	}
	// ensure nft was updated
	body := NftsResponse{}
	runtime.UnmarshallBody(&body, resp.Body, tc.r)
	assert.Len(body.Nfts, 1)
	assert.Empty(body.Error)
}
