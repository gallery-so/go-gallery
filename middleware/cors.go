package middleware

import (
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
	"strings"
)

func IsOriginAllowed(requestOrigin string) bool {
	allowedOrigins := strings.Split(viper.GetString("ALLOWED_ORIGINS"), ",")

	if util.ContainsString(allowedOrigins, requestOrigin) || (util.ContainsString([]string{"development", "sandbox-backend"}, strings.ToLower(viper.GetString("ENV"))) && strings.HasSuffix(requestOrigin, "-gallery-so.vercel.app")) {
		return true
	}

	return false
}
