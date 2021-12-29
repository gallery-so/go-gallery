package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestUpdateGalleryById_ReorderCollections_Success(t *testing.T) {
	assert := setupTest(t, 1)

	initialCollectionOrder := []persist.DBID{}

	// SET UP
	// Seed DB with collection
	for i := 0; i < 4; i++ {
		collID := createCollectionInDbForUserID(assert, fmt.Sprintf("Collection #%d", i), tc.user1.id)
		initialCollectionOrder = append(initialCollectionOrder, collID)
	}
	// Seed DB with gallery
	id, err := tc.repos.galleryRepository.Create(context.Background(), persist.GalleryDB{
		OwnerUserID: tc.user1.id,
		Collections: initialCollectionOrder,
	})
	assert.Nil(err)

	collIDS := []persist.DBID{}
	err = db.QueryRow(`SELECT COLLECTIONS FROM galleries WHERE ID = $1`, id).Scan(pq.Array(&collIDS))
	logrus.Infof("Collections in gallery: %v", collIDS)

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
	updateTestGallery(assert, update)

	// Validate the updated order of the gallery's collections
	validateCollectionsOrderInGallery(assert, updatedCollectionOrder)
}

// Retrieve the user's gallery and verify that the collections are in the expected order
func validateCollectionsOrderInGallery(assert *assert.Assertions, collections []persist.DBID) {
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
	logrus.Infof("Collections in gallery: %v", retreivedCollections)
}

func updateTestGallery(assert *assert.Assertions, update interface{}) {
	data, err := json.Marshal(update)
	assert.Nil(err)

	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/galleries/update", tc.serverURL),
		bytes.NewBuffer(data))
	assert.Nil(err)

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tc.user1.jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	assertValidResponse(assert, resp)
}
