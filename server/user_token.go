package server

import (
	"errors"
	"net/http"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/pubsub"
	"github.com/mikeydub/go-gallery/service/user"
	"github.com/mikeydub/go-gallery/util"
)

var errUserIDNotInCtx = errors.New("expected user ID to be in request context")

func updateUserInfo(userRepository persist.UserRepository, ethClient *eth.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := user.UpdateUserInput{}

		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := middleware.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		err := user.UpdateUser(c, userID, input, userRepository, ethClient)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})

	}
}

func getUser(userRepository persist.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := user.GetUserInput{}

		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		output, err := user.GetUser(
			c,
			input,
			userRepository,
		)
		if err != nil {
			status := http.StatusInternalServerError
			switch err.(type) {
			case persist.ErrUserNotFoundByAddress, persist.ErrUserNotFoundByID, persist.ErrUserNotFoundByUsername:
				status = http.StatusNotFound
			}
			util.ErrResponse(c, status, err)
			return
		}

		c.JSON(http.StatusOK, output)

	}
}

func createUserToken(userRepository persist.UserRepository, nonceRepository persist.NonceRepository, galleryRepository persist.GalleryTokenRepository, psub pubsub.PubSub, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := user.AddUserAddressesInput{}

		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		output, err := user.CreateUserToken(c, input, userRepository, nonceRepository, galleryRepository, ethClient, psub)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, output)

	}
}
func addUserAddress(userRepository persist.UserRepository, nonceRepository persist.NonceRepository, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := user.AddUserAddressesInput{}

		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := middleware.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		output, err := user.AddAddressToUser(c, userID, input, userRepository, nonceRepository, ethClient)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, output)

	}
}

func removeAddressesToken(userRepository persist.UserRepository, collRepo persist.CollectionTokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := user.RemoveUserAddressesInput{}

		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := middleware.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		err := user.RemoveAddressesFromUserToken(c, userID, input, userRepository, collRepo)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})

	}
}
