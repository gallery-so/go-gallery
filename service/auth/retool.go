package auth

import (
	"context"
	"errors"
	"github.com/mikeydub/go-gallery/env"
)

var errRetoolUnauthorized = errors.New("not authorized")

func RetoolAuthorized(ctx context.Context) error {
	if !BasicHeaderAuthorized(ctx, nil, env.GetString("RETOOL_AUTH_TOKEN")) {
		return errRetoolUnauthorized
	}

	return nil
}
