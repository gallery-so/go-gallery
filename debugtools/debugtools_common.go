// debugtools_common.go is always compiled and is not dependent on a build tag.
// It contains shared code used by both debugtools_enabled.go and debugtools_disabled.go.

package debugtools

import (
	"fmt"

	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
)

type DebugAuthenticator struct {
	UserID    persist.DBID
	Addresses []persist.AddressValue
	Chains    []persist.Chain
}

func (d DebugAuthenticator) GetDescription() string {
	return fmt.Sprintf("DebugAuthenticator(userId: %s, addresses: %v)", d.UserID, d.Addresses)
}

func NewDebugAuthenticator(userID persist.DBID, addresses []persist.AddressValue, chains []persist.Chain) auth.Authenticator {
	return DebugAuthenticator{
		UserID:    userID,
		Addresses: addresses,
		Chains:    chains,
	}
}
