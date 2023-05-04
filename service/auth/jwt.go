package auth

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/persist"
)

type authClaims struct {
	UserID persist.DBID `json:"user_id"`
	jwt.RegisteredClaims
}

// TODO: Add a source to this, and maybe add a type to all tokens?
type oneTimeLoginClaims struct {
	UserID persist.DBID `json:"user_id"`
	jwt.RegisteredClaims
}

type emailVerificationClaims struct {
	UserID persist.DBID `json:"user_id"`
	Email  string       `json:"email"`
	jwt.RegisteredClaims
}

func GenerateAuthToken(ctx context.Context, userID persist.DBID) (string, error) {
	secret := env.GetString("AUTH_JWT_SECRET")
	validFor := time.Duration(env.GetInt64("AUTH_JWT_TTL")) * time.Second

	claims := authClaims{
		UserID:           userID,
		RegisteredClaims: newRegisteredClaims(validFor),
	}

	return generateJWT(claims, secret)
}

func ParseAuthToken(ctx context.Context, token string) (persist.DBID, error) {
	claims := authClaims{}
	parsedToken, err := jwt.ParseWithClaims(token, &claims, keyFunc(env.GetString("AUTH_JWT_SECRET")))

	if err != nil || !parsedToken.Valid {
		return "", ErrInvalidJWT
	}

	return claims.UserID, nil
}

func GenerateOneTimeLoginToken(ctx context.Context, userID persist.DBID, validFor time.Duration) (string, error) {
	secret := env.GetString("ONE_TIME_LOGIN_JWT_SECRET")

	claims := oneTimeLoginClaims{
		UserID:           userID,
		RegisteredClaims: newRegisteredClaims(validFor),
	}

	return generateJWT(claims, secret)
}

func ParseOneTimeLoginToken(ctx context.Context, token string) (persist.DBID, time.Time, error) {
	claims := oneTimeLoginClaims{}
	parsedToken, err := jwt.ParseWithClaims(token, &claims, keyFunc(env.GetString("ONE_TIME_LOGIN_JWT_SECRET")))

	if err != nil || !parsedToken.Valid {
		return "", time.Time{}, ErrInvalidJWT
	}

	return claims.UserID, claims.ExpiresAt.Time, nil
}

func GenerateEmailVerificationToken(ctx context.Context, userID persist.DBID, email string) (string, error) {
	secret := env.GetString("EMAIL_VERIFICATION_JWT_SECRET")
	validFor := time.Duration(env.GetInt64("EMAIL_VERIFICATION_JWT_TTL")) * time.Second

	claims := emailVerificationClaims{
		UserID:           userID,
		Email:            email,
		RegisteredClaims: newRegisteredClaims(validFor),
	}

	return generateJWT(claims, secret)
}

func ParseEmailVerificationToken(ctx context.Context, token string) (persist.DBID, string, error) {
	claims := emailVerificationClaims{}
	parsedToken, err := jwt.ParseWithClaims(token, &claims, keyFunc(env.GetString("ONE_TIME_LOGIN_JWT_SECRET")))

	if err != nil || !parsedToken.Valid {
		return "", "", ErrInvalidJWT
	}

	return claims.UserID, claims.Email, nil
}

func newRegisteredClaims(validFor time.Duration) jwt.RegisteredClaims {
	claims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(validFor)),
		Issuer:    "gallery",
	}

	return claims
}

func generateJWT(claims jwt.Claims, jwtSecret string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	jwtToken, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}

	return jwtToken, nil
}

func keyFunc(secret string) jwt.Keyfunc {
	return func(*jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	}
}
