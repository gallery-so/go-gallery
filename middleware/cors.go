package middleware

import (
	"strings"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/util"
)

func IsOriginAllowed(requestOrigin string) bool {
	if env.GetString("ENV") == "local" {
		return true
	}
	allowedOrigins := strings.Split(env.GetString("ALLOWED_ORIGINS"), ",")

	if util.ContainsString(allowedOrigins, requestOrigin) || util.ContainsString([]string{"sandbox"}, strings.ToLower(env.GetString("ENV"))) || (util.ContainsString([]string{"development"}, strings.ToLower(env.GetString("ENV"))) && strings.HasSuffix(requestOrigin, "-gallery-so.vercel.app")) {
		return true
	}

	return false
}
