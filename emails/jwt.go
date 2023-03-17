package emails

import (
	"context"
	"errors"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/spf13/viper"
)

// this wouldn't be necessary if we were using go 1.18 because we could have the auth.ParseJWT function use generics to return the correct type

type jwtClaims struct {
	UserID persist.DBID `json:"user_id"`
	Email  string       `json:"email"`
	jwt.StandardClaims
}

var errInvalidJWT = errors.New("invalid JWT")

func jwtParse(pJWTtokenStr string) (persist.DBID, string, error) {

	claims := jwtClaims{}
	JWTtoken, err := jwt.ParseWithClaims(pJWTtokenStr,
		&claims,
		func(pJWTtoken *jwt.Token) (interface{}, error) {
			return []byte(env.Get[string](context.Background(), "JWT_SECRET")), nil
		})

	if err != nil || !JWTtoken.Valid {
		return "", "", errInvalidJWT
	}

	return claims.UserID, claims.Email, nil
}

func jwtGenerate(pUserID persist.DBID, email string) (string, error) {
	issuer := "gallery"

	signingKeyBytesLst := []byte(env.Get[string](context.Background(), "JWT_SECRET"))

	creationTimeUNIXint := time.Now().UnixNano() / 1000000000
	expiresAtUNIXint := creationTimeUNIXint + viper.GetInt64("JWT_TTL") // expire N number of secs from now
	claims := jwtClaims{
		pUserID,
		email,
		jwt.StandardClaims{
			ExpiresAt: expiresAtUNIXint,
			Issuer:    issuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	jwtTokenStr, err := token.SignedString(signingKeyBytesLst)
	if err != nil {
		return "", err
	}

	return jwtTokenStr, nil
}
