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

func TestUpdateCollectionById_Success(t *testing.T) {
	assert := assert.New(t)

	// seed DB with nft
	collId, err := persist.CollCreate(&persist.CollectionDb{
		NameStr:        "very cool collection",
		OwnerUserIDstr: tc.user1.id,
	}, context.Background(), tc.r)
	assert.Nil(err)

	// build update request body
	type Update struct {
		Id   persist.DbId `json:"id"`
		Name string       `json:"name"`
	}
	update := Update{Name: "new coll name", Id: collId}
	data, err := json.Marshal(update)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/collections/update/name", tc.serverUrl),
		bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertValidResponse(assert, resp)

	// retrieve updated nft
	resp, err = http.Get(fmt.Sprintf("%s/collections/get?user_id=%s", tc.serverUrl, tc.user1.id))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	type CollectionGetResponse struct {
		Collections []*persist.Collection `json:"collections"`
		Error       string                `json:"error"`
	}
	// ensure nft was updated
	body := CollectionGetResponse{}
	runtime.UnmarshalBody(&body, resp.Body, tc.r)
	assert.Len(body.Collections, 1)
	assert.Empty(body.Error)
	// assert.Equal(update.Name, body.Collections[0].NameStr)
}
