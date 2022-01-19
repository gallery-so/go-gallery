package server

import (
	"net/http"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

type authHasNFTInput struct {
	UserID persist.DBID `json:"user_id" form:"user_id" binding:"required"`
}

type authHasNFTOutput struct {
	HasNFT bool `json:"has_nft"`
}

func getAuthPreflight(userRepository persist.UserRepository, authNonceRepository persist.NonceRepository, ethClient *eth.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := auth.GetPreflightInput{}

		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		authed := c.GetBool(auth.AuthContextKey)

		output, err := auth.GetPreflight(c, input, authed, userRepository, authNonceRepository, ethClient)
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

		output, err := auth.LoginAndMemorizeAttempt(
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

		setJWTCookie(c, output.JWTtoken)

		c.JSON(http.StatusOK, output)
	}
}

func hasNFTs(userRepository persist.UserRepository, ethClient *eth.Client, tokenIDs []persist.TokenID) gin.HandlerFunc {
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
		for _, addr := range user.Addresses {
			if res, _ := ethClient.HasNFTs(c, tokenIDs, addr); res {
				has = true
				break
			}
		}
		c.JSON(http.StatusOK, authHasNFTOutput{HasNFT: has})
	}
}

func setJWTCookie(c *gin.Context, token string) {
	mode := http.SameSiteStrictMode
	if viper.GetString("ENV") != "production" {
		mode = http.SameSiteNoneMode
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     auth.JWTCookieKey,
		Value:    token,
		MaxAge:   viper.GetInt("JWT_TTL"),
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: mode,
	})
}
