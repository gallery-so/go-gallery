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
	"github.com/mikeydub/go-gallery/service/persist"
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
	wallets := make([]auth.AuthenticatedAddress, len(d.ChainAddresses))
	for i, chainAddress := range d.ChainAddresses {
		wallets[i] = auth.AuthenticatedAddress{
			ChainAddress: chainAddress,
			WalletType:   persist.WalletTypeEOA,
		}
	}

	authResult := auth.AuthResult{
		User:      d.User,
		Addresses: wallets,
	}

	return &authResult, nil
}
