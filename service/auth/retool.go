package auth

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/util"
)

var errRetoolUnauthorized = errors.New("not authorized")

func RetoolAuthorized(ctx context.Context) error {
	gc := util.GinContextFromContext(ctx)

	parts := strings.SplitN(gc.GetHeader("Authorization"), "Basic ", 2)
	if len(parts) != 2 {
		return errRetoolUnauthorized
	}

	usernameAndPassword, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return errRetoolUnauthorized
	}

	usernameAndPasswordParts := strings.SplitN(string(usernameAndPassword), ":", 2)
	if len(usernameAndPasswordParts) != 2 {
		return errRetoolUnauthorized
	}

	password := usernameAndPasswordParts[1]
	passwordBytes := []byte(password)

	if cmp := subtle.ConstantTimeCompare([]byte(env.GetString("RETOOL_AUTH_TOKEN")), passwordBytes); cmp != 1 {
		return errRetoolUnauthorized
	}

	return nil
}
