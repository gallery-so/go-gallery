package server

import (
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/membership"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

type getMembershipTiersResponse struct {
	Tiers []persist.MembershipTier `json:"tiers"`
}

func getMembershipTiers(membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, galleryRepository persist.GalleryRepository, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		allTiers, err := membershipRepository.GetAll(c)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		logrus.Debugf("Found %d membership tiers in the DB", len(allTiers))
		if len(allTiers) > 0 {
			if len(allTiers) != len(membership.MembershipTierIDs) {
				tiers := make(map[persist.TokenID]bool)
				for _, tier := range allTiers {
					tiers[tier.TokenID] = true
				}
				for _, tierID := range membership.MembershipTierIDs {
					if ok := tiers[tierID]; !ok {
						logrus.Infof("Tier not found - updating membership tier %s", tierID)
						newTier, err := membership.UpdateMembershipTier(tierID, membershipRepository, userRepository, galleryRepository, ethClient)
						if err != nil {
							util.ErrResponse(c, http.StatusInternalServerError, err)
							return
						}
						allTiers = append(allTiers, newTier)
					}
				}

			}

			for _, tier := range allTiers {
				if time.Since(tier.LastUpdated.Time()) > time.Hour {
					go membership.UpdateMembershipTier(tier.TokenID, membershipRepository, userRepository, galleryRepository, ethClient)
				}
			}

			c.JSON(http.StatusOK, getMembershipTiersResponse{Tiers: membership.OrderMembershipTiers(allTiers)})
			return
		}

		logrus.Infof("No tiers found - updating membership tiers")
		membershipTiers, err := membership.UpdateMembershipTiers(membershipRepository, userRepository, galleryRepository, ethClient)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, getMembershipTiersResponse{Tiers: membership.OrderMembershipTiers(membershipTiers)})
	}
}

func getMembershipTiersToken(membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, nftRepository persist.TokenRepository, galleryRepository persist.GalleryTokenRepository, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		allTiers, err := membershipRepository.GetAll(c)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		if len(allTiers) > 0 {
			if len(allTiers) != len(membership.MembershipTierIDs) {
				tiers := make(map[persist.TokenID]bool)

				for _, tier := range allTiers {
					tiers[tier.TokenID] = true
				}
				for _, tierID := range membership.MembershipTierIDs {
					if _, ok := tiers[tierID]; !ok {
						newTier, err := membership.UpdateMembershipTierToken(tierID, membershipRepository, userRepository, nftRepository, galleryRepository, ethClient)
						if err != nil {
							util.ErrResponse(c, http.StatusInternalServerError, err)
							return
						}
						allTiers = append(allTiers, newTier)
					}
				}
			}

			for _, tier := range allTiers {
				if time.Since(tier.LastUpdated.Time()) > time.Hour {
					go membership.UpdateMembershipTierToken(tier.TokenID, membershipRepository, userRepository, nftRepository, galleryRepository, ethClient)
				}
			}

			c.JSON(http.StatusOK, getMembershipTiersResponse{Tiers: membership.OrderMembershipTiers(allTiers)})
			return
		}
		membershipTiers, err := membership.UpdateMembershipTiersToken(membershipRepository, userRepository, nftRepository, galleryRepository, ethClient)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, getMembershipTiersResponse{Tiers: membership.OrderMembershipTiers(membershipTiers)})
	}
}
