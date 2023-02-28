package middleware

import (
	"strings"

	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

func IsOriginAllowed(requestOrigin string) bool {
	if viper.GetString("ENV") == "local" {
		return true
	}
	allowedOrigins := strings.Split(viper.GetString("ALLOWED_ORIGINS"), ",")

	if util.ContainsString(allowedOrigins, requestOrigin) || util.ContainsString([]string{"sandbox"}, strings.ToLower(viper.GetString("ENV"))) || (util.ContainsString([]string{"development"}, strings.ToLower(viper.GetString("ENV"))) && strings.HasSuffix(requestOrigin, "-gallery-so.vercel.app")) {
		return true
	}

	return false
}
