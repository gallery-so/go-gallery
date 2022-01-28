package server

import (
	"net/http"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/nft"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/pubsub"
	"github.com/mikeydub/go-gallery/service/user"
	"github.com/mikeydub/go-gallery/util"
)

func createUser(userRepository persist.UserRepository, nonceRepository persist.NonceRepository, galleryRepository persist.GalleryRepository, psub pubsub.PubSub, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := user.AddUserAddressesInput{}

		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		output, err := user.CreateUser(c, input, userRepository, nonceRepository, galleryRepository, ethClient)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		auth.SetJWTCookie(c, output.JWTtoken)

		c.JSON(http.StatusOK, output)

	}
}

func removeAddresses(userRepository persist.UserRepository, collRepo persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := user.RemoveUserAddressesInput{}

		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := auth.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		err := user.RemoveAddressesFromUser(c, userID, input, userRepository, collRepo)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})

	}
}

func getNFTPReviews(galleryRepository persist.GalleryRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := getPreviewsForUserInput{}

		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		output, err := nft.GetPreviewsForUser(c, galleryRepository, input.UserID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, getPreviewsForUserOutput{Previews: output})

	}
}
