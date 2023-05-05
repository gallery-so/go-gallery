package role

import (
	"context"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/persist"
	"strings"
)

type RoleFinder struct {
	Queries *db.Queries
}

func (r RoleFinder) RolesByUserID(ctx context.Context, userID persist.DBID) ([]persist.Role, error) {
	membershipAddress, memberTokens := parseAddressTokens(env.GetString("PREMIUM_CONTRACT_ADDRESS"))
	return r.Queries.GetUserRolesByUserId(ctx, db.GetUserRolesByUserIdParams{
		UserID:                userID,
		MembershipAddress:     persist.Address(membershipAddress),
		MembershipTokenIds:    memberTokens,
		GrantedMembershipRole: persist.RoleEarlyAccess, // Role granted if user carries a matching token
		Chain:                 persist.ChainETH,
	})
}

// parseAddressTokens returns a contract and tokens from a string encoded as '<address>=[<tokenID>,<tokenID>,...<tokenID>]'.
// It's helpful for parsing contract and tokens passed as environment variables.
func parseAddressTokens(s string) (string, []string) {
	addressTokens := strings.Split(s, "=")
	if len(addressTokens) != 2 {
		panic("invalid address tokens format")
	}
	address, tokens := addressTokens[0], addressTokens[1]
	tokens = strings.TrimLeft(tokens, "[")
	tokens = strings.TrimRight(tokens, "]")
	return address, strings.Split(tokens, ",")
}
