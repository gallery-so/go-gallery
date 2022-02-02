package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/assert"
)

func TestUpdateGalleryById_ReorderCollections_Success_Token(t *testing.T) {
	assert := setupTest(t, 2)

	initialCollectionOrder := []persist.DBID{}

	// SET UP
	// Seed DB with collection
	for i := 0; i < 4; i++ {
		collID := createCollectionInDbForUserIDToken(assert, fmt.Sprintf("Collection #%d", i), tc.user1.id)
		initialCollectionOrder = append(initialCollectionOrder, collID)
	}
	// Seed DB with gallery
	id, err := tc.repos.GalleryTokenRepository.Create(context.Background(), persist.GalleryTokenDB{
		OwnerUserID: tc.user1.id,
		Collections: initialCollectionOrder,
	})
	assert.Nil(err)

	// Validate the initial order of the gallery's collections
	validateCollectionsOrderInGallery(assert, initialCollectionOrder)

	// UPDATE COLLECTION ORDER
	// build update request body
	updatedCollectionOrder := []persist.DBID{
		initialCollectionOrder[3],
		initialCollectionOrder[2],
		initialCollectionOrder[1],
		initialCollectionOrder[0],
	}
	update := galleryTokenUpdateInput{Collections: updatedCollectionOrder, ID: id}
	updateTestGalleryToken(assert, update)

	time.Sleep(time.Second * 3)

	// Validate the updated order of the gallery's collections
	validateCollectionsOrderInGallery(assert, updatedCollectionOrder)
}

// Retrieve the user's gallery and verify that the collections are in the expected order
func validateCollectionsOrderInGalleryToken(assert *assert.Assertions, collections []persist.DBID) {
	getGalleryURL := fmt.Sprintf("%s/galleries/user_get?user_id=%s", tc.serverURL, tc.user1.id)
	resp, err := http.Get(getGalleryURL)
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	body := galleryTokenGetOutput{}
	util.UnmarshallBody(&body, resp.Body)
	assert.Len(body.Galleries, 1)
	retreivedCollections := body.Galleries[0].Collections

	for index, element := range collections {
		assert.Equal(element, retreivedCollections[index].ID)
	}
}

func updateTestGalleryToken(assert *assert.Assertions, update interface{}) {
	data, err := json.Marshal(update)
	assert.Nil(err)

	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/galleries/update", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)

	resp, err := tc.user1.client.Do(req)
	assert.Nil(err)
	assertValidResponse(assert, resp)
}
