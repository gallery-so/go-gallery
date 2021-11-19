package server

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
)

func createUser(userRepository persist.UserRepository, nonceRepository persist.NonceRepository, galleryRepository persist.GalleryRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &userAddAddressInput{}

		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		output, err := userCreateDb(c, input, userRepository, nonceRepository, galleryRepository)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, output)

	}
}

func removeAddresses(userRepository persist.UserRepository, collRepo persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &userRemoveAddressesInput{}

		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := middleware.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		err := removeAddressesFromUserDB(c, userID, input, userRepository, collRepo)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})

	}
}

func userCreateDb(pCtx context.Context, pInput *userAddAddressInput,
	userRepo persist.UserRepository, nonceRepo persist.NonceRepository, galleryRepo persist.GalleryRepository) (*userCreateOutput, error) {

	output := &userCreateOutput{}

	nonceValueStr, id, _ := getUserWithNonce(pCtx, pInput.Address, userRepo, nonceRepo)
	if nonceValueStr == "" {
		return nil, errNonceNotFound{pInput.Address}
	}
	if id != "" {
		return nil, errUserExistsWithAddress{address: pInput.Address}
	}

	sigValidBool, err := authVerifySignatureAllMethods(pInput.Signature,
		nonceValueStr,
		pInput.Address)
	if err != nil {
		return nil, err
	}

	output.SignatureValid = sigValidBool
	if !sigValidBool {
		return output, nil
	}

	user := &persist.User{
		Addresses: []persist.Address{pInput.Address},
	}

	userID, err := userRepo.Create(pCtx, user)
	if err != nil {
		return nil, err
	}

	output.UserID = userID

	jwtTokenStr, err := middleware.JWTGeneratePipeline(pCtx, userID)
	if err != nil {
		return nil, err
	}

	output.JWTtoken = jwtTokenStr

	err = authNonceRotateDb(pCtx, pInput.Address, userID, nonceRepo)
	if err != nil {
		return nil, err
	}

	galleryInsert := &persist.GalleryDB{OwnerUserID: userID, Collections: []persist.DBID{}}

	galleryID, err := galleryRepo.Create(pCtx, galleryInsert)
	if err != nil {
		return nil, err
	}

	output.GalleryID = galleryID

	return output, nil
}

func removeAddressesFromUserDB(pCtx context.Context, pUserID persist.DBID, pInput *userRemoveAddressesInput,
	userRepo persist.UserRepository, collRepo persist.CollectionRepository) error {

	user, err := userRepo.GetByID(pCtx, pUserID)
	if err != nil {
		return err
	}

	if len(user.Addresses) < len(pInput.Addresses) {
		return errUserCannotRemoveAllAddresses
	}

	err = userRepo.RemoveAddresses(pCtx, pUserID, pInput.Addresses)
	if err != nil {
		return err
	}
	return collRepo.RemoveNFTsOfAddresses(pCtx, pUserID, pInput.Addresses)
}
