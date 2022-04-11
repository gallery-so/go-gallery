package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

type getCommunityInput struct {
	ContractAddress persist.Address `form:"contract_address" binding:"required"`
}

type getCommunityOutput struct {
	Community persist.Community `json:"community"`
}

func getCommunity(communityRepo persist.CommunityRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input getCommunityInput
		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		community, err := communityRepo.GetByAddress(c, input.ContractAddress)
		if err != nil {
			if _, ok := err.(persist.ErrCommunityNotFound); ok {
				util.ErrResponse(c, http.StatusNotFound, err)
				return
			}
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, getCommunityOutput{Community: community})
	}
}
