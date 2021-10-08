package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/spf13/viper"
)

type jwtClaims struct {
	UserID persist.DBID `json:"user_id"`
	jwt.StandardClaims
}

type jwtValidateResponse struct {
	IsValid bool         `json:"valid"`
	UserID  persist.DBID `json:"user_id"`
}

func validateJwt() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetBool(authContextKey)
		userID := getUserIDfromCtx(c)

		c.JSON(http.StatusOK, jwtValidateResponse{
			IsValid: auth,
			UserID:  userID,
		})
	}
}

func authJwtParse(pJWTtokenStr string,
	pJWTsecretKeyStr string) (bool, persist.DBID, error) {

	claims := jwtClaims{}
	JWTtoken, err := jwt.ParseWithClaims(pJWTtokenStr,
		&claims,
		func(pJWTtoken *jwt.Token) (interface{}, error) {
			return []byte(pJWTsecretKeyStr), nil
		})

	if err != nil {
		return false, "", err
	}

	if !JWTtoken.Valid {
		return false, "", errors.New("JWT token is invalid")
	}

	return true, claims.UserID, nil
}

func jwtGeneratePipeline(pCtx context.Context, pUserID persist.DBID) (string, error) {

	issuer := "gallery"
	jwtTokenStr, err := jwtGenerate(issuer, pUserID)
	if err != nil {
		return "", err
	}

	return jwtTokenStr, nil
}

func jwtGenerate(
	pIssuerStr string,
	pUserID persist.DBID) (string, error) {

	signingKeyBytesLst := []byte(viper.GetString("JWT_SECRET"))

	creationTimeUNIXint := time.Now().UnixNano() / 1000000000
	expiresAtUNIXint := creationTimeUNIXint + viper.GetInt64("JWT_TTL") //60*60*24*2 // expire N number of secs from now
	claims := jwtClaims{
		pUserID,
		jwt.StandardClaims{
			ExpiresAt: expiresAtUNIXint,
			Issuer:    pIssuerStr,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	jwtTokenStr, err := token.SignedString(signingKeyBytesLst)
	if err != nil {
		return "", err
	}

	return jwtTokenStr, nil
}
