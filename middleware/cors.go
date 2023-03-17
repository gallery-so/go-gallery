package middleware

import (
	"context"
	"strings"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/util"
)

func IsOriginAllowed(requestOrigin string) bool {
	if env.Get[string](context.Background(), "ENV") == "local" {
		return true
	}
	allowedOrigins := strings.Split(env.Get[string](context.Background(), "ALLOWED_ORIGINS"), ",")

	if util.ContainsString(allowedOrigins, requestOrigin) || util.ContainsString([]string{"sandbox"}, strings.ToLower(env.Get[string](context.Background(), "ENV"))) || (util.ContainsString([]string{"development"}, strings.ToLower(env.Get[string](context.Background(), "ENV"))) && strings.HasSuffix(requestOrigin, "-gallery-so.vercel.app")) {
		return true
	}

	return false
}
