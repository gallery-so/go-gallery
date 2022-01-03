package server

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

type galleryGetByUserIDInput struct {
	UserID persist.DBID `form:"user_id" json:"user_id" binding:"required"`
}
type galleryGetByIDInput struct {
	ID persist.DBID `form:"id" json:"id" binding:"required"`
}

type galleryGetByIDOutput struct {
	Gallery persist.Gallery `json:"gallery"`
}

type galleryUpdateInput struct {
	ID          persist.DBID   `form:"id" json:"id" binding:"required"`
	Collections []persist.DBID `json:"collections" binding:"required"`
}

type galleryGetOutput struct {
	Galleries []persist.Gallery `json:"galleries"`
}

type galleryRefreshCacheInput struct {
	UserID persist.DBID `json:"user_id" binding:"required"`
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

		galleries, err := galleryRepository.GetByUserID(c, input.UserID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		if galleries == nil {
			galleries = []persist.Gallery{}
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

		gallery, err := galleryRepository.GetByID(c, input.ID)
		if err != nil {
			status := http.StatusInternalServerError
			if _, ok := err.(persist.ErrGalleryNotFoundByID); ok {
				status = http.StatusNotFound
			}
			util.ErrResponse(c, status, err)
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

		update := persist.GalleryUpdateInput{Collections: input.Collections}

		err := galleryRepository.Update(c, input.ID, userID, update)
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

func refreshGallery(galleryRepository persist.GalleryRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &galleryRefreshCacheInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		if err := galleryRepository.RefreshCache(c, input.UserID); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}
