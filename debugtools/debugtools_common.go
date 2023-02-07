// debugtools_common.go is always compiled and is not dependent on a build tag.
// It contains shared code used by both debugtools_enabled.go and debugtools_disabled.go.

package debugtools

import (
	"fmt"

	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/socialauth"
)

type DebugAuthenticator struct {
	User           *persist.User
	ChainAddresses []persist.ChainAddress
}

func (d DebugAuthenticator) GetDescription() string {
	return fmt.Sprintf("DebugAuthenticator(user: %+v, addresses: %v)", d.User, d.ChainAddresses)
}

func NewDebugAuthenticator(user *persist.User, chainAddresses []persist.ChainAddress) auth.Authenticator {
	return DebugAuthenticator{
		User:           user,
		ChainAddresses: chainAddresses,
	}
}

type DebugSocialAuthenticator struct {
	Provider persist.SocialProvider
	ID       string
	Metadata map[string]interface{}
}

func NewDebugSocialAuthenticator(provider persist.SocialProvider, id string, metadata map[string]interface{}) socialauth.Authenticator {
	return DebugSocialAuthenticator{
		Provider: provider,
		ID:       id,
		Metadata: metadata,
	}
}
