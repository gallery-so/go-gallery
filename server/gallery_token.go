package server

import (
	"fmt"
	"net/http"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"

	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

type galleryTokenGetByUserIDInput struct {
	UserID persist.DBID `form:"user_id" json:"user_id" binding:"required"`
}
type galleryTokenGetByIDInput struct {
	ID persist.DBID `form:"id" json:"id" binding:"required"`
}

type galleryTokenGetByIDOutput struct {
	Gallery persist.GalleryToken `json:"gallery"`
}

type galleryTokenUpdateInput struct {
	ID          persist.DBID   `form:"id" json:"id" binding:"required"`
	Collections []persist.DBID `json:"collections" binding:"required"`
}

type galleryTokenGetOutput struct {
	Galleries []persist.GalleryToken `json:"galleries"`
}

type errNoGalleriesFoundWithID struct {
	id persist.DBID
}

// HANDLERS

func getGalleriesByUserIDToken(galleryRepository persist.GalleryTokenRepository, tokenRepository persist.TokenRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client, storageClient *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &galleryTokenGetByUserIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		galleries, err := galleryRepository.GetByUserID(c, input.UserID)
		if len(galleries) == 0 || err != nil {
			galleries = []persist.GalleryToken{}
		}

		c.JSON(http.StatusOK, galleryTokenGetOutput{Galleries: galleries})

	}
}

func getGalleryByIDToken(galleryRepository persist.GalleryTokenRepository, tokenRepository persist.TokenRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client, storageClient *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &galleryTokenGetByIDInput{}
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

		c.JSON(http.StatusOK, galleryTokenGetByIDOutput{Gallery: gallery})
		return

	}
}

func updateGalleryToken(galleryRepository persist.GalleryTokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &galleryTokenUpdateInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := middleware.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		update := persist.GalleryTokenUpdateInput{Collections: input.Collections}

		err := galleryRepository.Update(c, input.ID, userID, update)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func (e errNoGalleriesFoundWithID) Error() string {
	return fmt.Sprintf("no galleries found with id: %s", e.id)
}
