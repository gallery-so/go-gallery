package server

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
)

type galleryGetByUserIDInput struct {
	UserID persist.DBID `form:"user_id" json:"user_id" binding:"required"`
}
type galleryGetByIDInput struct {
	ID persist.DBID `form:"id" json:"id" binding:"required"`
}

type galleryGetByIDOutput struct {
	Gallery *persist.Gallery `json:"gallery"`
}

type galleryUpdateInput struct {
	ID          persist.DBID   `form:"id" json:"id" binding:"required"`
	Collections []persist.DBID `json:"collections" binding:"required"`
}

type galleryGetOutput struct {
	Galleries []*persist.Gallery `json:"galleries"`
}

// HANDLERS

func getGalleriesByUserID(galleryRepository persist.GalleryRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &galleryGetByUserIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		auth := c.GetBool(middleware.AuthContextKey)
		galleries, err := galleryRepository.GetByUserID(c, input.UserID, auth)
		if len(galleries) == 0 || err != nil {
			galleries = []*persist.Gallery{}
		}

		c.JSON(http.StatusOK, galleryGetOutput{Galleries: galleries})

	}
}

func getGalleryByID(galleryRepository persist.GalleryRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &galleryGetByIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		auth := c.GetBool(middleware.AuthContextKey)
		gallery, err := galleryRepository.GetByID(c, input.ID, auth)
		if err != nil {
			status := http.StatusInternalServerError
			if _, ok := err.(persist.ErrGalleryNotFoundByID); ok {
				status = http.StatusNotFound
			}
			c.JSON(status, util.ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, galleryGetByIDOutput{Gallery: gallery})
		return

	}
}

func updateGallery(galleryRepository persist.GalleryRepository, backupRepository persist.BackupRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &galleryUpdateInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := middleware.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		update := &persist.GalleryUpdateInput{Collections: input.Collections}

		err := galleryRepository.Update(c, input.ID, userID, update)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		go func(ctx context.Context) {
			galleries, err := galleryRepository.GetByUserID(ctx, userID, true)
			if err == nil {
				for _, gallery := range galleries {
					backupRepository.Insert(ctx, gallery)
				}
			}

		}(c.Copy())

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}
