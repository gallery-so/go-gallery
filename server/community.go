package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/community"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

type getCommunityInput struct {
	ContractAddress persist.Address `uri:"address"`
}
type getCommunityOutput struct {
	Community persist.Community `json:"community"`
}
type getCommunitiesOutput struct {
	Communities []persist.Community `json:"communities"`
}

func getAllCommunities(communityRepository persist.CommunityRepository, nftRepository persist.NFTRepository, galleryRepository persist.GalleryRepository, userRepository persist.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		communities, err := communityRepository.GetAll(c)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, getCommunitiesOutput{Communities: communities})

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
		defer cancel()
		if len(communities) != len(community.Communities) {
			if err := community.UpdateCommunities(ctx, communityRepository, galleryRepository, userRepository, nftRepository); err != nil {
				logrus.WithError(err).Error("Error updating communities")
			}
			return
		}
		for _, com := range communities {
			if time.Since(com.LastUpdated.Time()) > time.Hour*24 {
				go community.UpdateCommunity(ctx, com.ContractAddress, nftRepository, userRepository, galleryRepository, communityRepository)
			}
		}
	}
}

func getCommunity(communityRepository persist.CommunityRepository, nftRepository persist.NFTRepository, galleryRepository persist.GalleryRepository, userRepository persist.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input getCommunityInput
		if err := c.ShouldBindUri(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		com, err := communityRepository.GetByContract(c, input.ContractAddress)
		if err != nil {
			if _, ok := err.(persist.ErrCommunityNotFoundByAddress); ok {
				err := community.UpdateCommunity(c, input.ContractAddress, nftRepository, userRepository, galleryRepository, communityRepository)
				if err != nil {
					util.ErrResponse(c, http.StatusInternalServerError, err)
					return
				}
				com, err = communityRepository.GetByContract(c, input.ContractAddress)
				if err != nil {
					util.ErrResponse(c, http.StatusInternalServerError, err)
					return
				}
				c.JSON(http.StatusOK, getCommunityOutput{Community: com})
				return
			}
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, getCommunityOutput{Community: com})
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
		defer cancel()
		if time.Since(com.LastUpdated.Time()) > time.Hour*24 {
			err := community.UpdateCommunity(ctx, input.ContractAddress, nftRepository, userRepository, galleryRepository, communityRepository)
			if err != nil {
				logrus.WithError(err).Errorf("Error updating community: %s", input.ContractAddress)
			}
		}
	}
}
