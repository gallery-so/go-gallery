package basicauth

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/util"
)

type AuthTokenType string

const (
	AuthTokenTypeRetool     AuthTokenType = "Retool"
	AuthTokenTypeMonitoring AuthTokenType = "Monitoring"
)

// AuthorizeHeader checks if the request has a Basic Auth header matching the specified
// username and password. Username is optional and will be ignored if nil.
func AuthorizeHeader(ctx context.Context, username *string, password string) bool {
	gc := util.MustGetGinContext(ctx)
	headerUsername, headerPassword, ok := gc.Request.BasicAuth()
	if !ok {
		return false
	}

	// Username is optional, but has to match if not nil
	if username != nil {
		if cmp := subtle.ConstantTimeCompare([]byte(*username), []byte(headerUsername)); cmp != 1 {
			return false
		}
	}

	// Password is required
	if cmp := subtle.ConstantTimeCompare([]byte(password), []byte(headerPassword)); cmp != 1 {
		return false
	}

	return true
}

// AuthorizeHeaderForAllowedTypes checks whether the request has a Basic Auth header matching one of the
// known token types. If the request has a valid token, it returns true. Otherwise, it returns false.
func AuthorizeHeaderForAllowedTypes(ctx context.Context, allowedTypes []AuthTokenType) bool {
	authTokens := map[AuthTokenType]string{
		AuthTokenTypeRetool:     env.GetString("BASIC_AUTH_TOKEN_RETOOL"),
		AuthTokenTypeMonitoring: env.GetString("BASIC_AUTH_TOKEN_MONITORING"),
	}

	for _, authType := range allowedTypes {
		authToken, ok := authTokens[authType]
		if !ok {
			logger.For(ctx).Errorf("Basic auth: unknown type %s", authType)
			continue
		}

		if AuthorizeHeader(ctx, nil, authToken) {
			return true
		}
	}

	return false
}

// MakeHeader takes a password and an optional username and base64 encodes them in the
// format required for a basic auth header. The output can be used as the value for the
// "Authorization" header.
func MakeHeader(username *string, password string) string {
	usernameValue := ""
	if username != nil {
		usernameValue = *username
	}

	data := usernameValue + ":" + password
	encoded := base64.StdEncoding.EncodeToString([]byte(data))
	return "Basic " + encoded
}
