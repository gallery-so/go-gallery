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

func TestUpdateGalleryById_Success(t *testing.T) {
	assert := assert.New(t)

	colls := []persist.DBID{}

	for i := 0; i < 10; i++ {
		col := &persist.CollectionDB{Name: "asdad", OwnerUserID: tc.user1.id, CollectorsNote: "yee"}
		if i == 3 {
			col.Hidden = true
		}
		id, err := persist.CollCreate(context.TODO(), col, tc.r)
		assert.Nil(err)
		colls = append(colls, id)
	}

	t.Log(colls)

	// seed DB with collection
	id, err := persist.GalleryCreate(context.Background(), &persist.GalleryDB{
		OwnerUserID: tc.user1.id,
		Collections: colls,
	}, tc.r)
	assert.Nil(err)

	// build update request body
	type Update struct {
		ID          persist.DBID   `json:"id"`
		Collections []persist.DBID `json:"collections"`
	}

	copy := colls
	hold := copy[1]
	copy[1] = copy[2]
	copy[2] = hold

	t.Log(copy)

	update := Update{Collections: copy, ID: id}
	data, err := json.Marshal(update)
	assert.Nil(err)

	// send update request
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/galleries/update", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertValidResponse(assert, resp)

	// retrieve updated gallery
	getURL := fmt.Sprintf("%s/galleries/user_get?user_id=%s", tc.serverURL, tc.user1.id)
	t.Log(getURL)
	resp, err = http.Get(getURL)
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	type GalleryGetResponse struct {
		Galleries []*persist.Gallery `json:"galleries"`
		Error     string             `json:"error"`
	}
	// ensure nft was updated
	body := GalleryGetResponse{}
	runtime.UnmarshalBody(&body, resp.Body, tc.r)
	assert.Len(body.Galleries, 1)
	assert.Empty(body.Error)
	assert.Equal(update.Collections[2], body.Galleries[0].Collections[1].ID)
	assert.Len(body.Galleries[0].Collections, 9)
}
