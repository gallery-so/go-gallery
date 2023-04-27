package auth

import (
	"context"
	"errors"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/auth/basicauth"
)

var errRetoolUnauthorized = errors.New("not authorized")

func RetoolAuthorized(ctx context.Context) error {
	if !basicauth.AuthorizeHeader(ctx, nil, env.GetString("RETOOL_AUTH_TOKEN")) {
		return errRetoolUnauthorized
	}

	return nil
}
