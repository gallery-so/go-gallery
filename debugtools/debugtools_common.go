// debugtools_common.go is always compiled and is not dependent on a build tag.
// It contains shared code used by both debugtools_enabled.go and debugtools_disabled.go.

package debugtools

import (
	"fmt"

	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
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
