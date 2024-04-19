package membership

import (
	"context"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
)

// MembershipTierIDs is a list of all membership tiers
var MembershipTierIDs = []persist.HexTokenID{"4", "1", "3", "5", "6", "8"}

// OrderMembershipTiers orders the membership tiers in the desired order determined for the membership page
func OrderMembershipTiers(pTiers []persist.MembershipTier) []persist.MembershipTier {
	result := make([]persist.MembershipTier, 0, len(pTiers))
	for _, v := range MembershipTierIDs {
		for _, t := range pTiers {
			if t.TokenID == v {
				result = append(result, t)
			}
		}
	}
	return result
}

// GetMembershipTiers returns the most recent membership tiers and potentially updates tiers
func GetMembershipTiers(ctx context.Context, membershipRepository *postgres.MembershipRepository) ([]persist.MembershipTier, error) {
	allTiers, err := membershipRepository.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	logger.For(ctx).Debugf("Found %d membership tiers in the DB", len(allTiers))
	return OrderMembershipTiers(allTiers), nil
}
