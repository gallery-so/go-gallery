//go:build !debug_tools
// +build !debug_tools

// debugtools_disabled.go is compiled whenever the `debug_tools` build tag is not set.
// It should provide production-safe alternative implementations of the code found in
// debugtools_enabled.go.

package debugtools

import (
	"context"
	"errors"

	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/socialauth"
)

const Enabled bool = false

func (d DebugAuthenticator) Authenticate(ctx context.Context) (*auth.AuthResult, error) {
	return nil, errors.New("DebugAuthenticator only works when the 'debug_tools' build tag is set")
}

func (d DebugSocialAuthenticator) Authenticate(ctx context.Context) (*socialauth.SocialAuthResult, error) {
	return nil, errors.New("DebugSocialAuthenticator only works when the 'debug_tools' build tag is set")
}
