//go:build debug_tools
// +build debug_tools

// debugtools_enabled.go is only compiled when the `debug_tools` build tag is set.
// Anything that should be debug-only can be added here. Additionally, because the
// 'Enabled' bool is a const, code in other files that is conditional on Enabled
// will also be compiled out of builds.

package debugtools

import (
	"context"
	"errors"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/spf13/viper"
)

const Enabled bool = true

func init() {
	// An additional safeguard against running debug tools in production
	if viper.GetString("ENV") == "production" {
		panic(errors.New("debug tools may not be enabled in a production environment"))
	}
}

func (d DebugAuthenticator) Authenticate(ctx context.Context) (*auth.AuthResult, error) {
	if viper.GetString("ENV") != "local" {
		return nil, errors.New("DebugAuthenticator may only be used in a local environment")
	}

	authResult := auth.AuthResult{
		Addresses: d.Addresses,
		UserID:    d.UserID,
	}

	return &authResult, nil
}
