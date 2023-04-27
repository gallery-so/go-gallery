package basicauth

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"github.com/mikeydub/go-gallery/util"
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
