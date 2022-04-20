package server

import (
	"net/http"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

type authHasNFTInput struct {
	UserID persist.DBID `json:"user_id" form:"user_id" binding:"required"`
}

type authHasNFTOutput struct {
	HasNFT bool `json:"has_nft"`
}

func getAuthPreflight(userRepository persist.UserRepository, authNonceRepository persist.NonceRepository, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := auth.GetPreflightInput{}

		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		authed := auth.GetUserAuthedFromCtx(c)

		output, err := auth.GetAuthNonceREST(c, input, authed, userRepository, authNonceRepository, ethClient)
		if err != nil {
			status := http.StatusInternalServerError
			if _, ok := err.(persist.ErrNonceNotFoundForAddress); ok {
				status = http.StatusNotFound
			}
			util.ErrResponse(c, status, err)
			return
		}

		c.JSON(http.StatusOK, output)
	}
}

func login(userRepository persist.UserRepository, authNonceRepository persist.NonceRepository, authLoginRepository persist.LoginAttemptRepository, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := auth.LoginInput{}
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		output, err := auth.LoginREST(
			c,
			input,
			c.Request,
			userRepository,
			authNonceRepository,
			authLoginRepository,
			ethClient,
		)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, output)
	}
}

func logout() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth.SetJWTCookie(c, "")
		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func hasNFTs(userRepository persist.UserRepository, ethClient *ethclient.Client, contractAddress persist.EthereumAddress, tokenIDs []persist.TokenID) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &authHasNFTInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		user, err := userRepository.GetByID(c, input.UserID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
			return
		}
		has := false
		for _, addr := range user.Wallets {
			if addr.Chain != persist.ChainETH {
				continue
			}
			if res, _ := eth.HasNFTs(c, contractAddress, tokenIDs, persist.EthereumAddress(addr.Address.Address), ethClient); res {
				has = true
				break
			}
		}
		c.JSON(http.StatusOK, authHasNFTOutput{HasNFT: has})
	}
}
