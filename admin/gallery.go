package admin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

var errGetGalleriesInput = errors.New("id or user_id must be provided")

type getGalleriesInput struct {
	ID     persist.DBID `form:"id"`
	UserID persist.DBID `form:"user_id"`
}

func getGalleries(galleryRepo persist.GalleryRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		var input getGalleriesInput
		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		if input.ID == "" && input.UserID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errGetGalleriesInput)
			return
		}

		var galleries []persist.Gallery
		var err error

		if input.ID == "" {
			gallery, e := galleryRepo.GetByID(c, input.ID)
			galleries = []persist.Gallery{gallery}
			err = e
		} else {
			galleries, err = galleryRepo.GetByUserID(c, input.UserID)
		}
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, galleries)
	}
}
