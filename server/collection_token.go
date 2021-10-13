package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"google.golang.org/appengine"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
)

var errTooManyTokensInCollection = errors.New("a collection can have no more than 1000 tokens")

type collectionGetByUserIDInputToken struct {
	UserID persist.DBID `form:"user_id" json:"user_id" binding:"required"`
}
type collectionGetByIDInputToken struct {
	ID persist.DBID `form:"id" json:"id" binding:"required"`
}
type collectionGetByIDOutputToken struct {
	Collection *persist.CollectionToken `json:"collection"`
}

type collectionGetOutputtoken struct {
	Collections []*persist.CollectionToken `json:"collections"`
}

type collectionCreateInputToken struct {
	GalleryID      persist.DBID   `json:"gallery_id" binding:"required"`
	Nfts           []persist.DBID `json:"nfts" binding:"required"`
	Name           string         `json:"name"`
	CollectorsNote string         `json:"collectors_note"`
}

type collectionUpdateInfoByIDInputToken struct {
	ID             persist.DBID `json:"id" binding:"required"`
	Name           string       `json:"name" binding:"required"`
	CollectorsNote string       `json:"collectors_note"`
}

type collectionUpdateHiddenByIDInputToken struct {
	ID     persist.DBID `json:"id" binding:"required"`
	Hidden bool         `json:"hidden"`
}
type collectionUpdateNftsByIDInputToken struct {
	ID   persist.DBID   `json:"id" binding:"required"`
	Nfts []persist.DBID `json:"nfts" binding:"required"`
}

type collectionCreateOutputToken struct {
	ID persist.DBID `json:"collection_id"`
}

type collectionDeleteInputToken struct {
	ID persist.DBID `json:"id" binding:"required"`
}

type errNoCollectionsFoundWithID struct {
	id persist.DBID
}

// HANDLERS

func getCollectionsByUserIDToken(collectionsRepository persist.CollectionTokenRepository, tokenRepository persist.TokenRepository, ipfsClient *shell.Shell) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &collectionGetByUserIDInputToken{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		userID := getUserIDfromCtx(c)
		auth := userID == input.UserID
		colls, err := collectionsRepository.GetByUserID(c, input.UserID, auth)
		if len(colls) == 0 || err != nil {
			colls = []*persist.CollectionToken{}
		}

		aeCtx := appengine.NewContext(c.Request)
		for _, coll := range colls {
			coll.Nfts = ensureCollectionTokenMedia(aeCtx, coll.Nfts, tokenRepository, ipfsClient)
		}

		c.JSON(http.StatusOK, collectionGetOutputtoken{Collections: colls})

	}
}

func getCollectionByIDToken(collectionsRepository persist.CollectionTokenRepository, tokenRepository persist.TokenRepository, ipfsClient *shell.Shell) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &collectionGetByIDInputToken{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		auth := c.GetBool(authContextKey)
		coll, err := collectionsRepository.GetByID(c, input.ID, auth)
		if err != nil {
			c.JSON(http.StatusNotFound, util.ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		coll.Nfts = ensureCollectionTokenMedia(appengine.NewContext(c.Request), coll.Nfts, tokenRepository, ipfsClient)

		c.JSON(http.StatusOK, collectionGetByIDOutputToken{Collection: coll})
		return

	}
}

func createCollectionToken(collectionsRepository persist.CollectionTokenRepository, galleryRepository persist.GalleryTokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &collectionCreateInputToken{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		userID := getUserIDfromCtx(c)
		if userID == "" {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: errUserIDNotInCtx.Error()})
			return
		}

		//------------------
		// CREATE

		id, err := collectionCreateDbToken(c, input, userID, collectionsRepository, galleryRepository)
		if err != nil {
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, collectionCreateOutputToken{ID: id})
	}
}

func updateCollectionInfoToken(collectionsRepository persist.CollectionTokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateInfoByIDInputToken{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: err.Error()})
			return
		}

		userID := getUserIDfromCtx(c)
		if userID == "" {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: errUserIDNotInCtx.Error()})
			return
		}

		update := &persist.CollectionTokenUpdateInfoInput{Name: sanitizationPolicy.Sanitize(input.Name), CollectorsNote: sanitizationPolicy.Sanitize(input.CollectorsNote)}

		err := collectionsRepository.Update(c, input.ID, userID, update)
		if err != nil {
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func updateCollectionHiddenToken(collectionsRepository persist.CollectionTokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateHiddenByIDInputToken{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: err.Error()})
			return
		}

		userID := getUserIDfromCtx(c)
		if userID == "" {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: errUserIDNotInCtx.Error()})
			return
		}

		update := &persist.CollectionTokenUpdateHiddenInput{Hidden: input.Hidden}

		err := collectionsRepository.Update(c, input.ID, userID, update)
		if err != nil {
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func updateCollectionTokensToken(collectionsRepository persist.CollectionTokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateNftsByIDInputToken{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: err.Error()})
			return
		}

		// TODO magic number
		if len(input.Nfts) > 1000 {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: errTooManyTokensInCollection.Error()})
			return
		}

		userID := getUserIDfromCtx(c)
		if userID == "" {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: errUserIDNotInCtx.Error()})
			return
		}

		// ensure that there are no repeat NFTs
		withNoRepeats := uniqueDBID(input.Nfts)

		update := &persist.CollectionTokenUpdateNftsInput{Nfts: withNoRepeats}

		err := collectionsRepository.UpdateNFTs(c, input.ID, userID, update)
		if err != nil {
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func deleteCollectionToken(collectionsRepository persist.CollectionTokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionDeleteInputToken{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		userID := getUserIDfromCtx(c)
		if userID == "" {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: errUserIDNotInCtx.Error()})
			return
		}

		err := collectionsRepository.Delete(c, input.ID, userID)
		if err != nil {
			c.JSON(http.StatusNotFound, util.ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

// CREATE
func collectionCreateDbToken(pCtx context.Context, pInput *collectionCreateInputToken,
	pUserID persist.DBID,
	collectionsRepo persist.CollectionTokenRepository, galleryRepo persist.GalleryTokenRepository) (persist.DBID, error) {

	coll := &persist.CollectionTokenDB{
		OwnerUserID:    pUserID,
		Nfts:           pInput.Nfts,
		Name:           sanitizationPolicy.Sanitize(pInput.Name),
		CollectorsNote: sanitizationPolicy.Sanitize(pInput.CollectorsNote),
	}

	collID, err := collectionsRepo.Create(pCtx, coll)
	if err != nil {
		return "", err
	}

	err = galleryRepo.AddCollections(pCtx, pInput.GalleryID, pUserID, []persist.DBID{collID})
	if err != nil {
		return "", err
	}

	return collID, nil

}

// uniqueDBID ensures that an array of DBIDs has no repeat items
func uniqueDBID(a []persist.DBID) []persist.DBID {
	result := []persist.DBID{}
	m := map[persist.DBID]bool{}

	for _, val := range a {
		if _, ok := m[val]; !ok {
			m[val] = true
			result = append(result, val)
		}
	}

	return result
}

func (e errNoCollectionsFoundWithID) Error() string {
	return fmt.Sprintf("no collections found with ID %s", e.id)
}
