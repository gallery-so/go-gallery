package auth

import (
	"context"
	"crypto/subtle"
	"github.com/mikeydub/go-gallery/util"
)

// BasicHeaderAuthorized checks if the request has a Basic Auth header matching the specified
// username and password. Username is optional and will be ignored if nil.
func BasicHeaderAuthorized(ctx context.Context, username *string, password string) bool {
	gc := util.GinContextFromContext(ctx)
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
