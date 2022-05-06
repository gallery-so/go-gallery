package server

import (
	"github.com/mikeydub/go-gallery/service/logger"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/membership"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

type getMembershipTiersInput struct {
	ForceRefresh bool `form:"refresh"`
}

type getMembershipTiersResponse struct {
	Tiers []persist.MembershipTier `json:"tiers"`
}

func getMembershipTiersREST(membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, galleryRepository persist.GalleryRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input getMembershipTiersInput
		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		membershipTiers, err := membership.GetMembershipTiers(c, input.ForceRefresh, membershipRepository, userRepository, galleryRepository, ethClient, ipfsClient, arweaveClient, storageClient)

		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, getMembershipTiersResponse{Tiers: membershipTiers})
	}
}

func getMembershipTiersToken(membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, nftRepository persist.TokenRepository, galleryRepository persist.GalleryTokenRepository, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input getMembershipTiersInput
		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		if input.ForceRefresh {
			logger.For(c).Infof("Force refresh - updating membership tiers")
		}
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

			tiersToUpdate := make([]persist.TokenID, 0, len(allTiers))
			for _, tier := range allTiers {
				if time.Since(tier.LastUpdated.Time()) > time.Hour || input.ForceRefresh {
					logger.For(c).Infof("Tier %s not updated in the last hour - updating membership tier", tier.TokenID)
					tiersToUpdate = append(tiersToUpdate, tier.TokenID)
				}
			}
			if len(tiersToUpdate) > 0 {
				go func() {
					for _, tierID := range tiersToUpdate {
						_, err := membership.UpdateMembershipTierToken(tierID, membershipRepository, userRepository, nftRepository, galleryRepository, ethClient)
						if err != nil {
							logger.For(c).WithError(err).Errorf("Failed to update membership tier %s", tierID)
						}
					}
				}()
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
