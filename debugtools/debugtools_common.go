// debugtools_common.go is always compiled and is not dependent on a build tag.
// It contains shared code used by both debugtools_enabled.go and debugtools_disabled.go.

package debugtools

import (
	"fmt"

	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
)

type DebugAuthenticator struct {
	UserID         persist.DBID
	ChainAddresses []persist.ChainAddress
}

func (d DebugAuthenticator) GetDescription() string {
	return fmt.Sprintf("DebugAuthenticator(userId: %s, addresses: %v)", d.UserID, d.ChainAddresses)
}

func NewDebugAuthenticator(userID persist.DBID, chainAddresses []persist.ChainAddress) auth.Authenticator {
	return DebugAuthenticator{
		UserID:         userID,
		ChainAddresses: chainAddresses,
	}
}
