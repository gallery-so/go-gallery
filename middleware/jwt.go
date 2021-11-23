package middleware

import (
	"context"
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

// JWTValidateResponse is the response for the jwt validation endpoint
type JWTValidateResponse struct {
	IsValid bool         `json:"valid"`
	UserID  persist.DBID `json:"user_id"`
}

// ValidateJWT is a handler that validates the JWT token and returns the user ID
func ValidateJWT() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetBool(AuthContextKey)
		userID := GetUserIDFromCtx(c)

		c.JSON(http.StatusOK, JWTValidateResponse{
			IsValid: auth,
			UserID:  userID,
		})
	}
}

// AuthJWTParse parses the JWT token from the request and returns whether the token is valid and the user ID associated with it
func AuthJWTParse(pJWTtokenStr string,
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
		return false, "", ErrInvalidJWT
	}

	return true, claims.UserID, nil
}

// JWTGeneratePipeline generates a new JWT token for the user
func JWTGeneratePipeline(pCtx context.Context, pUserID persist.DBID) (string, error) {

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
