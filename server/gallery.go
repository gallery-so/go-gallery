package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
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

func getGalleriesByUserID(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &galleryGetByUserIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		auth := c.GetBool(authContextKey)
		galleries, err := persist.GalleryGetByUserID(c, input.UserID, auth, pRuntime)
		if len(galleries) == 0 || err != nil {
			galleries = []*persist.Gallery{}
		}

		c.JSON(http.StatusOK, galleryGetOutput{Galleries: galleries})

	}
}

func getGalleryByID(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &galleryGetByIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		auth := c.GetBool(authContextKey)
		galleries, err := persist.GalleryGetByID(c, input.ID, auth, pRuntime)
		if len(galleries) == 0 || err != nil {
			c.JSON(http.StatusNotFound, util.ErrorResponse{
				Error: fmt.Sprintf("no galleries found with id: %s", input.ID),
			})
			return
		}
		if len(galleries) > 1 {
			galleries = galleries[:1]
			// TODO log that this should not be happening
		}

		c.JSON(http.StatusOK, galleryGetByIDOutput{Gallery: galleries[0]})
		return

	}
}

func updateGallery(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &galleryUpdateInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: err.Error()})
			return
		}

		userID, ok := getUserIDfromCtx(c)
		if !ok {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: "user id not found in context"})
			return
		}

		update := &persist.GalleryUpdateInput{Collections: input.Collections}

		err := persist.GalleryUpdate(c, input.ID, userID, update, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}
