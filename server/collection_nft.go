package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

var errTooManyNFTsInCollection = errors.New("maximum of 1000 NFTs in a collection")

type collectionGetByUserIDInput struct {
	UserID persist.DBID `form:"user_id" json:"user_id" binding:"required"`
}
type collectionGetByIDInput struct {
	ID persist.DBID `form:"id" json:"id" binding:"required"`
}
type collectionGetByIDOutput struct {
	Collection persist.Collection `json:"collection"`
}

type collectionGetOutput struct {
	Collections []persist.Collection `json:"collections"`
}

type collectionCreateInput struct {
	GalleryID      persist.DBID        `json:"gallery_id" binding:"required"`
	Nfts           []persist.DBID      `json:"nfts" binding:"required"`
	Layout         persist.TokenLayout `json:"layout"`
	Name           string              `json:"name"`
	CollectorsNote string              `json:"collectors_note" binding:"collectors_note"`
}

type collectionUpdateInfoByIDInput struct {
	ID             persist.DBID `json:"id" binding:"required"`
	Name           string       `json:"name"`
	CollectorsNote string       `json:"collectors_note" binding:"collectors_note"`
}

type collectionUpdateHiddenByIDInput struct {
	ID     persist.DBID `json:"id" binding:"required"`
	Hidden bool         `json:"hidden"`
}
type collectionUpdateNftsByIDInput struct {
	ID     persist.DBID        `json:"id" binding:"required"`
	Nfts   []persist.DBID      `json:"nfts" binding:"required"`
	Layout persist.TokenLayout `json:"layout"`
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

		input := &collectionGetByUserIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := auth.GetUserIDFromCtx(c)
		auth := userID == input.UserID
		colls, err := collectionsRepository.GetByUserID(c, input.UserID, auth)
		if len(colls) == 0 || err != nil {
			colls = []persist.Collection{}
		}

		c.JSON(http.StatusOK, collectionGetOutput{Collections: colls})

	}
}

func getCollectionByID(collectionsRepository persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &collectionGetByIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		auth := c.GetBool(auth.AuthContextKey)
		coll, err := collectionsRepository.GetByID(c, input.ID, auth)
		if err != nil {
			status := http.StatusInternalServerError
			if _, ok := err.(persist.ErrCollectionNotFoundByID); ok {
				status = http.StatusNotFound
			}
			util.ErrResponse(c, status, err)
			return
		}

		c.JSON(http.StatusOK, collectionGetByIDOutput{Collection: coll})
		return

	}
}

//------------------------------------------------------------

func createCollection(collectionsRepository persist.CollectionRepository, galleryRepository persist.GalleryRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := collectionCreateInput{}
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := auth.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		id, err := collectionCreateDb(c, input, userID, collectionsRepository, galleryRepository)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, collectionCreateOutput{ID: id})
	}
}

func updateCollectionInfo(collectionsRepository persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateInfoByIDInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := auth.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		update := persist.CollectionUpdateInfoInput{Name: persist.NullString(validate.SanitizationPolicy.Sanitize(input.Name)), CollectorsNote: persist.NullString(validate.SanitizationPolicy.Sanitize(input.CollectorsNote))}

		err := collectionsRepository.Update(c, input.ID, userID, update)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func updateCollectionHidden(collectionsRepository persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateHiddenByIDInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := auth.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		update := persist.CollectionUpdateHiddenInput{Hidden: persist.NullBool(input.Hidden)}

		err := collectionsRepository.Update(c, input.ID, userID, update)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func updateCollectionNfts(collectionsRepository persist.CollectionRepository, galleryRepository persist.GalleryRepository, backupRepository persist.BackupRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateNftsByIDInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		// TODO magic number
		if len(input.Nfts) > 1000 {
			util.ErrResponse(c, http.StatusBadRequest, errTooManyNFTsInCollection)
			return
		}

		userID := auth.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		// ensure that there are no repeat NFTs
		withNoRepeats := uniqueDBID(input.Nfts)
		layout, err := persist.ValidateLayout(input.Layout)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		update := persist.CollectionUpdateNftsInput{NFTs: withNoRepeats, Layout: layout}

		err = collectionsRepository.UpdateNFTs(c, input.ID, userID, update)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		go func(ctx context.Context) {
			galleries, err := galleryRepository.GetByUserID(ctx, userID)
			if err == nil {
				for _, gallery := range galleries {
					backupRepository.Insert(ctx, gallery)
				}
			}
		}(c.Copy())

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func deleteCollection(collectionsRepository persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionDeleteInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := auth.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		err := collectionsRepository.Delete(c, input.ID, userID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

// CREATE
func collectionCreateDb(pCtx context.Context, pInput collectionCreateInput,
	pUserID persist.DBID,
	collectionsRepo persist.CollectionRepository, galleryRepo persist.GalleryRepository) (persist.DBID, error) {

	layout, err := persist.ValidateLayout(pInput.Layout)
	if err != nil {
		return "", err
	}
	coll := persist.CollectionDB{
		OwnerUserID:    pUserID,
		NFTs:           pInput.Nfts,
		Layout:         layout,
		Name:           persist.NullString(validate.SanitizationPolicy.Sanitize(pInput.Name)),
		CollectorsNote: persist.NullString(validate.SanitizationPolicy.Sanitize(pInput.CollectorsNote)),
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
