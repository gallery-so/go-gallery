package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
)

var (
	jwtTTL    int64
	jwtSecret string
)

// JWT_CLAIMS
type jwtClaims struct {
	UserID persist.DBID `json:"user_id"`
	jwt.StandardClaims
}

type jwtValidateResponse struct {
	IsValid bool         `json:"valid"`
	UserID  persist.DBID `json:"user_id"`
}

// HANDLER

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

// VERIFY
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
	jwtTokenStr, err := jwtGenerate(jwtSecret, issuer, pUserID)
	if err != nil {
		return "", err
	}

	return jwtTokenStr, nil
}

// GENERATE
// ADD!! - make sure when creating new JWT tokens for user that the old ones are marked as deleted

func jwtGenerate(pSigningKeyStr string,
	pIssuerStr string,
	pUserID persist.DBID) (string, error) {

	signingKeyBytesLst := []byte(pSigningKeyStr)

	//------------------
	// CLAIMS

	// Create the Claims
	creationTimeUNIXint := time.Now().UnixNano() / 1000000000
	expiresAtUNIXint := creationTimeUNIXint + jwtTTL //60*60*24*2 // expire N number of secs from now
	claims := jwtClaims{
		pUserID,
		jwt.StandardClaims{
			ExpiresAt: expiresAtUNIXint,
			Issuer:    pIssuerStr,
		},
	}

	//------------------

	// SYMETRIC_SIGNING - same secret is used to both sign and validate tokens
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// SIGN
	jwtTokenStr, err := token.SignedString(signingKeyBytesLst)
	if err != nil {

		return "", err
	}

	return jwtTokenStr, nil
}
