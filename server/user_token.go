package server

import (
	"errors"
	"net/http"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/nft"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/user"
	"github.com/mikeydub/go-gallery/util"
)

var errUserIDNotInCtx = errors.New("expected user ID to be in request context")

type getPreviewsForUserOutput struct {
	Previews []persist.NullString `json:"previews"`
}

func updateUserInfo(userRepository persist.UserRepository, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := user.UpdateUserInput{}

		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := auth.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		err := user.UpdateUser(c, userID, input.UserName, input.BioStr, userRepository, ethClient)
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
			case persist.ErrUserNotFound:
				status = http.StatusNotFound
			}
			util.ErrResponse(c, status, err)
			return
		}

		c.JSON(http.StatusOK, output)

	}
}

func getCurrentUser(userRepository persist.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		authed := auth.GetUserAuthedFromCtx(c)
		if !authed {
			err := auth.GetAuthErrorFromCtx(c)
			if err != http.ErrNoCookie {
				auth.SetJWTCookie(c, "")
			}
			c.JSON(http.StatusNoContent, util.SuccessResponse{Success: false})
			return
		}
		userID := auth.GetUserIDFromCtx(c)

		output, err := user.GetUser(
			c,
			user.GetUserInput{UserID: userID},
			userRepository,
		)
		if err != nil {
			status := http.StatusInternalServerError
			switch err.(type) {
			case persist.ErrUserNotFound:
				auth.SetJWTCookie(c, "")
				status = http.StatusNotFound
			}
			util.ErrResponse(c, status, err)
			return
		}

		c.JSON(http.StatusOK, output)
	}
}

func createUserToken(userRepository persist.UserRepository, nonceRepository persist.NonceRepository, galleryRepository persist.GalleryTokenRepository, tokenRepo persist.TokenRepository, contractRepo persist.ContractRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := user.AddUserAddressesInput{}

		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		output, err := user.CreateUserToken(c, input, userRepository, nonceRepository, galleryRepository, tokenRepo, contractRepo, ethClient, ipfsClient, arweaveClient, stg)
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

		userID := auth.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		authenticator := auth.EthereumNonceAuthenticator{
			Address:    input.Address,
			Nonce:      input.Nonce,
			Signature:  input.Signature,
			WalletType: input.WalletType,
			UserRepo:   userRepository,
			NonceRepo:  nonceRepository,
			EthClient:  ethClient,
		}

		err := user.AddAddressToUser(c, userID, input.Address, authenticator, userRepository)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		output := user.AddUserAddressOutput{
			SignatureValid: true,
		}

		c.JSON(http.StatusOK, output)

	}
}
func addUserAddressToken(userRepository persist.UserRepository, nonceRepository persist.NonceRepository, tokenRepo persist.TokenRepository, contractRepo persist.ContractRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := user.AddUserAddressesInput{}

		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := auth.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		authenticator := auth.EthereumNonceAuthenticator{
			Address:    input.Address,
			Nonce:      input.Nonce,
			Signature:  input.Signature,
			WalletType: input.WalletType,
			UserRepo:   userRepository,
			NonceRepo:  nonceRepository,
			EthClient:  ethClient,
		}

		err := user.AddAddressToUserToken(c, userID, input.Address, authenticator, userRepository, tokenRepo, contractRepo, ethClient, ipfsClient, arweaveClient, stg)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		output := user.AddUserAddressOutput{
			SignatureValid: true,
		}

		c.JSON(http.StatusOK, output)

	}
}

func removeAddressesToken(userRepository persist.UserRepository) gin.HandlerFunc {
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

		err := user.RemoveAddressesFromUserToken(c, userID, input, userRepository)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})

	}
}

func getNFTPreviewsToken(galleryRepository persist.GalleryTokenRepository, userRepository persist.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := nft.GetPreviewsForUserInput{}

		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		output, err := nft.GetPreviewsForUserToken(c, galleryRepository, userRepository, input)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, getPreviewsForUserOutput{Previews: output})

	}
}
func mergeUsers(userRepository persist.UserRepository, nonceRepository persist.NonceRepository, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input user.MergeUsersInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		userID := auth.GetUserIDFromCtx(c)

		if err := user.MergeUsers(c, userRepository, nonceRepository, userID, input, ethClient); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}
