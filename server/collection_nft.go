package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
)

var errTooManyNFTsInCollection = errors.New("maximum of 1000 NFTs in a collection")

type collectionGetByUserIDInput struct {
	UserID persist.DBID `form:"user_id" json:"user_id" binding:"required"`
}
type collectionGetByIDInput struct {
	ID persist.DBID `form:"id" json:"id" binding:"required"`
}
type collectionGetByIDOutput struct {
	Collection *persist.Collection `json:"collection"`
}

type collectionGetOutput struct {
	Collections []*persist.Collection `json:"collections"`
}

type collectionCreateInput struct {
	GalleryID      persist.DBID   `json:"gallery_id" binding:"required"`
	Nfts           []persist.DBID `json:"nfts" binding:"required"`
	Name           string         `json:"name"`
	CollectorsNote string         `json:"collectors_note"`
}

type collectionUpdateInfoByIDInput struct {
	ID             persist.DBID `json:"id" binding:"required"`
	Name           string       `json:"name" binding:"required"`
	CollectorsNote string       `json:"collectors_note"`
}

type collectionUpdateHiddenByIDInput struct {
	ID     persist.DBID `json:"id" binding:"required"`
	Hidden bool         `json:"hidden"`
}
type collectionUpdateNftsByIDInput struct {
	ID   persist.DBID   `json:"id" binding:"required"`
	Nfts []persist.DBID `json:"nfts" binding:"required"`
}

type collectionCreateOutput struct {
	ID persist.DBID `json:"collection_id"`
}

type collectionDeleteInput struct {
	ID persist.DBID `json:"id" binding:"required"`
}

// HANDLERS

func getCollectionsByUserID(collectionsRepository persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &collectionGetByUserIDInput{}
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
			colls = []*persist.Collection{}
		}

		c.JSON(http.StatusOK, collectionGetOutput{Collections: colls})

	}
}

func getCollectionByID(collectionsRepository persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &collectionGetByIDInput{}
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

		c.JSON(http.StatusOK, collectionGetByIDOutput{Collection: coll})
		return

	}
}

//------------------------------------------------------------

func createCollection(collectionsRepository persist.CollectionRepository, galleryRepository persist.GalleryRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &collectionCreateInput{}
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

		id, err := collectionCreateDb(c, input, userID, collectionsRepository, galleryRepository)
		if err != nil {
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, collectionCreateOutput{ID: id})
	}
}

func updateCollectionInfo(collectionsRepository persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateInfoByIDInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: err.Error()})
			return
		}

		userID := getUserIDfromCtx(c)
		if userID == "" {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: errUserIDNotInCtx.Error()})
			return
		}

		update := &persist.CollectionUpdateInfoInput{Name: sanitizationPolicy.Sanitize(input.Name), CollectorsNote: sanitizationPolicy.Sanitize(input.CollectorsNote)}

		err := collectionsRepository.Update(c, input.ID, userID, update)
		if err != nil {
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func updateCollectionHidden(collectionsRepository persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateHiddenByIDInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: err.Error()})
			return
		}

		userID := getUserIDfromCtx(c)
		if userID == "" {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: errUserIDNotInCtx.Error()})
			return
		}

		update := &persist.CollectionUpdateHiddenInput{Hidden: input.Hidden}

		err := collectionsRepository.Update(c, input.ID, userID, update)
		if err != nil {
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func updateCollectionNfts(collectionsRepository persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateNftsByIDInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: err.Error()})
			return
		}

		// TODO magic number
		if len(input.Nfts) > 1000 {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: errTooManyNFTsInCollection.Error()})
			return
		}

		userID := getUserIDfromCtx(c)
		if userID == "" {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: errUserIDNotInCtx.Error()})
			return
		}

		// ensure that there are no repeat NFTs
		withNoRepeats := uniqueDBID(input.Nfts)

		update := &persist.CollectionUpdateNftsInput{Nfts: withNoRepeats}

		err := collectionsRepository.UpdateNFTs(c, input.ID, userID, update)
		if err != nil {
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func deleteCollection(collectionsRepository persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionDeleteInput{}
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
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

// CREATE
func collectionCreateDb(pCtx context.Context, pInput *collectionCreateInput,
	pUserID persist.DBID,
	collectionsRepo persist.CollectionRepository, galleryRepo persist.GalleryRepository) (persist.DBID, error) {

	coll := &persist.CollectionDB{
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
