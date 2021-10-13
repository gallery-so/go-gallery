package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"google.golang.org/appengine"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
)

type galleryTokenGetByUserIDInput struct {
	UserID persist.DBID `form:"user_id" json:"user_id" binding:"required"`
}
type galleryTokenGetByIDInput struct {
	ID persist.DBID `form:"id" json:"id" binding:"required"`
}

type galleryTokenGetByIDOutput struct {
	Gallery *persist.GalleryToken `json:"gallery"`
}

type galleryTokenUpdateInput struct {
	ID          persist.DBID   `form:"id" json:"id" binding:"required"`
	Collections []persist.DBID `json:"collections" binding:"required"`
}

type galleryTokenGetOutput struct {
	Galleries []*persist.GalleryToken `json:"galleries"`
}

type errNoGalleriesFoundWithID struct {
	id persist.DBID
}

// HANDLERS

func getGalleriesByUserIDToken(galleryRepository persist.GalleryTokenRepository, tokenRepository persist.TokenRepository, ipfsClient *shell.Shell) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &galleryTokenGetByUserIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		auth := c.GetBool(authContextKey)
		galleries, err := galleryRepository.GetByUserID(c, input.UserID, auth)
		if len(galleries) == 0 || err != nil {
			galleries = []*persist.GalleryToken{}
		}
		aeCtx := appengine.NewContext(c.Request)
		for _, gallery := range galleries {
			for _, collection := range gallery.Collections {
				collection.Nfts = ensureCollectionTokenMedia(aeCtx, collection.Nfts, tokenRepository, ipfsClient)
			}
		}

		c.JSON(http.StatusOK, galleryTokenGetOutput{Galleries: galleries})

	}
}

func getGalleryByIDToken(galleryRepository persist.GalleryTokenRepository, tokenRepository persist.TokenRepository, ipfsClient *shell.Shell) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &galleryTokenGetByIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		auth := c.GetBool(authContextKey)
		galleries, err := galleryRepository.GetByID(c, input.ID, auth)
		if len(galleries) == 0 || err != nil {
			c.JSON(http.StatusNotFound, util.ErrorResponse{
				Error: errNoGalleriesFoundWithID{id: input.ID}.Error(),
			})
			return
		}
		if len(galleries) > 1 {
			galleries = galleries[:1]
			// TODO log that this should not be happening
		}
		gallery := galleries[0]
		aeCtx := appengine.NewContext(c.Request)
		for _, collection := range gallery.Collections {
			collection.Nfts = ensureCollectionTokenMedia(aeCtx, collection.Nfts, tokenRepository, ipfsClient)
		}

		c.JSON(http.StatusOK, galleryTokenGetByIDOutput{Gallery: galleries[0]})
		return

	}
}

func updateGalleryToken(galleryRepository persist.GalleryTokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &galleryTokenUpdateInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: err.Error()})
			return
		}

		userID := getUserIDfromCtx(c)
		if userID == "" {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: errUserIDNotInCtx.Error()})
			return
		}

		update := &persist.GalleryTokenUpdateInput{Collections: input.Collections}

		err := galleryRepository.Update(c, input.ID, userID, update)
		if err != nil {
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func (e errNoGalleriesFoundWithID) Error() string {
	return fmt.Sprintf("no galleries found with id: %s", e.id)
}
