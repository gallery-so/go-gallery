package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

type galleryGetByUserIdInput struct {
	UserId persist.DbID `form:"user_id" json:"user_id" binding:"required"`
}
type galleryGetByIdInput struct {
	Id persist.DbID `form:"id" json:"id" binding:"required"`
}

type galleryCreateInput struct {
	OwnerUserID persist.DbID `form:"owner_user_id" json:"owner_user_id" binding:"required"`
}

type galleryUpdateInput struct {
	Id          persist.DbID   `form:"id" json:"id" binding:"required"`
	Collections []persist.DbID `json:"collections" binding:"required"`
}

type galleryGetOutput struct {
	Galleries []*persist.Gallery `json:"galleries"`
}

type galleryCreateOutput struct {
	Id persist.DbID `json:"id"`
}

//-------------------------------------------------------------
// HANDLERS

func getGalleriesByUserId(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &galleryGetByUserIdInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{
				Error: err.Error(),
			})
			return
		}

		auth := c.GetBool(authContextKey)
		galleries, err := persist.GalleryGetByUserID(c, input.UserId, auth, pRuntime)
		if len(galleries) == 0 || err != nil {
			galleries = []*persist.Gallery{}
		}

		c.JSON(http.StatusOK, galleryGetOutput{Galleries: galleries})

	}
}

//-------------------------------------------------------------
func getGalleryById(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &galleryGetByIdInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{
				Error: err.Error(),
			})
			return
		}

		auth := c.GetBool(authContextKey)
		galleries, err := persist.GalleryGetByID(c, input.Id, auth, pRuntime)
		if len(galleries) == 0 || err != nil {
			c.JSON(http.StatusNotFound, errorResponse{
				Error: fmt.Sprintf("no galleries found with id: %s", input.Id),
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

		err := persist.GalleryUpdate(input.Id, userID, update, c, pRuntime)
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

		c.JSON(http.StatusOK, galleryCreateOutput{Id: id})
	}
}
