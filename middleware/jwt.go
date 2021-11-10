package middleware

import (
	"context"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/spf13/viper"
)

type jwtClaims struct {
	UserID persist.DBID `json:"user_id"`
	jwt.StandardClaims
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
		return false, "", ErrInvalidJWT
	}

	return true, claims.UserID, nil
}

// JWTGenerate generates a JWT token for the given userID
func JWTGenerate(pCtx context.Context, pUserID persist.DBID) (string, error) {

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
