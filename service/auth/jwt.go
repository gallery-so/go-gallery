package auth

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/persist"
)

type TokenType string

const (
	TokenTypeAuth              TokenType = "auth"
	TokenTypeRefresh           TokenType = "refresh"
	TokenTypeOneTimeLogin      TokenType = "one_time_login"
	TokenTypeEmailVerification TokenType = "email_verification"
)

type GalleryClaims struct {
	TokenType TokenType `json:"token_type"`
	jwt.RegisteredClaims
}

type AuthTokenClaims struct {
	UserID    persist.DBID   `json:"user_id"`
	SessionID persist.DBID   `json:"session_id"` // The session this auth token belongs to
	RefreshID string         `json:"refresh_id"` // The refresh token this auth token was generated from
	Roles     []persist.Role `json:"roles"`
	GalleryClaims
}

type RefreshTokenClaims struct {
	ID        string       `json:"id"`        // The refresh token's ID
	ParentID  string       `json:"parent_id"` // The parent refresh token this child refresh token was generated from
	UserID    persist.DBID `json:"user_id"`
	SessionID persist.DBID `json:"session_id"` // The session this refresh token belongs to
	GalleryClaims
}

type oneTimeLoginClaims struct {
	UserID persist.DBID `json:"user_id"`
	Source string       `json:"source"`
	GalleryClaims
}

type emailVerificationClaims struct {
	UserID persist.DBID `json:"user_id"`
	Email  string       `json:"email"`
	GalleryClaims
}

func GenerateAuthToken(ctx context.Context, userID persist.DBID, sessionID persist.DBID, refreshID string, roles []persist.Role) (string, error) {
	secret := env.GetString("AUTH_JWT_SECRET")
	validFor := time.Duration(env.GetInt64("AUTH_JWT_TTL")) * time.Second

	claims := AuthTokenClaims{
		UserID:        userID,
		SessionID:     sessionID,
		RefreshID:     refreshID,
		Roles:         roles,
		GalleryClaims: newGalleryClaims(TokenTypeAuth, validFor),
	}

	return generateJWT(claims, secret)
}

func ParseAuthToken(ctx context.Context, token string) (AuthTokenClaims, error) {
	claims := AuthTokenClaims{}
	parsedToken, err := jwt.ParseWithClaims(token, &claims, keyFunc(env.GetString("AUTH_JWT_SECRET")))

	if err != nil || !parsedToken.Valid {
		return AuthTokenClaims{}, ErrInvalidJWT
	}

	return claims, nil
}

func GenerateRefreshToken(ctx context.Context, ID string, parentID string, userID persist.DBID, sessionID persist.DBID) (string, time.Time, error) {
	secret := env.GetString("REFRESH_JWT_SECRET")
	validFor := time.Duration(env.GetInt64("REFRESH_JWT_TTL")) * time.Second

	claims := RefreshTokenClaims{
		ID:            ID,
		ParentID:      parentID,
		UserID:        userID,
		SessionID:     sessionID,
		GalleryClaims: newGalleryClaims(TokenTypeRefresh, validFor),
	}

	jwt, err := generateJWT(claims, secret)
	expiresAt := time.Now().Add(validFor)

	return jwt, expiresAt, err
}

func ParseRefreshToken(ctx context.Context, token string) (RefreshTokenClaims, error) {
	claims := RefreshTokenClaims{}
	parsedToken, err := jwt.ParseWithClaims(token, &claims, keyFunc(env.GetString("REFRESH_JWT_SECRET")))

	if err != nil || !parsedToken.Valid {
		return RefreshTokenClaims{}, ErrInvalidJWT
	}

	return claims, nil
}

func GenerateOneTimeLoginToken(ctx context.Context, userID persist.DBID, source string, validFor time.Duration) (string, error) {
	secret := env.GetString("ONE_TIME_LOGIN_JWT_SECRET")

	claims := oneTimeLoginClaims{
		UserID:        userID,
		Source:        source,
		GalleryClaims: newGalleryClaims(TokenTypeOneTimeLogin, validFor),
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
		UserID:        userID,
		Email:         email,
		GalleryClaims: newGalleryClaims(TokenTypeEmailVerification, validFor),
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

func newGalleryClaims(tokenType TokenType, validFor time.Duration) GalleryClaims {
	claims := GalleryClaims{
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(validFor)),
			Issuer:    "gallery",
		},
	}

	return claims
}

func generateJWT(claims jwt.Claims, jwtSecret string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	jwtToken, err := token.SignedString([]byte(jwtSecret))
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
