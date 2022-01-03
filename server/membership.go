package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/membership"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

type getMembershipTiersResponse struct {
	Tiers []persist.MembershipTier `json:"tiers"`
}

func getMembershipTiers(membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, nftRepository persist.NFTRepository, ethClient *eth.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		allTiers, err := membershipRepository.GetAll(c)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		if len(allTiers) > 0 {
			updatedRecently := true
			for _, tier := range allTiers {
				if time.Since(tier.LastUpdated.Time()) > time.Hour {
					updatedRecently = false
					break
				}
			}
			if !updatedRecently {
				go membership.UpdateMembershipTiers(membershipRepository, userRepository, nftRepository, ethClient)
			}
			c.JSON(http.StatusOK, getMembershipTiersResponse{Tiers: allTiers})
			return
		}

		membershipTiers, err := membership.UpdateMembershipTiers(membershipRepository, userRepository, nftRepository, ethClient)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, getMembershipTiersResponse{Tiers: membershipTiers})
	}
}

func getMembershipTiersToken(membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, nftRepository persist.TokenRepository, ethClient *eth.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		allTiers, err := membershipRepository.GetAll(c)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		if len(allTiers) > 0 {
			updatedRecently := true
			for _, tier := range allTiers {
				if time.Since(tier.LastUpdated.Time()) > time.Hour {
					updatedRecently = false
					break
				}
			}
			if !updatedRecently {
				go membership.UpdateMembershipTiersToken(membershipRepository, userRepository, nftRepository, ethClient)
			}
			c.JSON(http.StatusOK, getMembershipTiersResponse{Tiers: allTiers})
			return
		}
		membershipTiers, err := membership.UpdateMembershipTiersToken(membershipRepository, userRepository, nftRepository, ethClient)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, getMembershipTiersResponse{Tiers: membershipTiers})
	}
}
