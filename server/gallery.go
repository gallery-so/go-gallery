package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

type galleryGetByUserIDInput struct {
	UserID persist.DbID `form:"user_id" json:"user_id" binding:"required"`
}
type galleryGetByIDInput struct {
	ID persist.DbID `form:"id" json:"id" binding:"required"`
}

type galleryCreateInput struct {
	OwnerUserID persist.DbID `form:"owner_user_id" json:"owner_user_id" binding:"required"`
}

type galleryUpdateInput struct {
	ID          persist.DbID   `form:"id" json:"id" binding:"required"`
	Collections []persist.DbID `json:"collections" binding:"required"`
}

type galleryGetOutput struct {
	Galleries []*persist.Gallery `json:"galleries"`
}

type galleryCreateOutput struct {
	ID persist.DbID `json:"id"`
}

//-------------------------------------------------------------
// HANDLERS

func getGalleriesByUserID(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &galleryGetByUserIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{
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

//-------------------------------------------------------------
func getGalleryByID(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &galleryGetByIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{
				Error: err.Error(),
			})
			return
		}

		auth := c.GetBool(authContextKey)
		galleries, err := persist.GalleryGetByID(c, input.ID, auth, pRuntime)
		if len(galleries) == 0 || err != nil {
			c.JSON(http.StatusNotFound, errorResponse{
				Error: fmt.Sprintf("no galleries found with id: %s", input.ID),
			})
			return
		}
		if len(galleries) > 1 {
			galleries = galleries[:1]
			// TODO log that this should not be happening
		}

		c.JSON(http.StatusOK, galleryGetOutput{Galleries: galleries})
		return

	}
}

//-------------------------------------------------------------

func updateGallery(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &galleryUpdateInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		userID, ok := getUserIDfromCtx(c)
		if !ok {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "user id not found in context"})
			return
		}

		update := &persist.GalleryUpdateInput{Collections: input.Collections}

		err := persist.GalleryUpdate(input.ID, userID, update, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, successOutput{Success: true})
	}
}

//-------------------------------------------------------------

func createGallery(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &galleryCreateInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		userID, ok := getUserIDfromCtx(c)
		if !ok {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "user id not found in context"})
			return
		}

		insert := &persist.GalleryDb{OwnerUserID: userID}

		id, err := persist.GalleryCreate(c, insert, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, galleryCreateOutput{ID: id})
	}
}
