package admin

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

var errGetNFTsInput = errors.New("address or user_id must be provided")

type getNFTsInput struct {
	Address persist.Address `form:"address"`
	UserID  persist.DBID    `form:"user_id"`
}

// RefreshNFTsInput is the input for the refreshOpensea function
type RefreshNFTsInput struct {
	UserIDs   []persist.DBID    `json:"user_ids"`
	Addresses []persist.Address `json:"addresses"`
}

func getNFTs(nftRepo persist.NFTRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input getNFTsInput
		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		if input.Address == "" && input.UserID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errGetNFTsInput)
			return
		}

		var nfts []persist.NFT
		var err error

		if input.Address == "" {
			nfts, err = nftRepo.GetByUserID(c, input.UserID)
		} else {
			nfts, err = nftRepo.GetByAddresses(c, []persist.Address{input.Address})
		}
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, nfts)
	}
}

func refreshOpensea(nftRepo persist.NFTRepository, userRepo persist.UserRepository, collRepo persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input RefreshNFTsInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		if len(input.UserIDs) == 0 && len(input.Addresses) == 0 {
			util.ErrResponse(c, http.StatusBadRequest, errGetNFTsInput)
			return
		}
		err := RefreshOpensea(c, input, userRepo, nftRepo, collRepo)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

// RefreshOpensea refreshes the opensea data for the given user ids and addresses
func RefreshOpensea(c context.Context, input RefreshNFTsInput, userRepo persist.UserRepository, nftRepo persist.NFTRepository, collRepo persist.CollectionRepository) error {
	logrus.Debugf("refreshOpensea input: %+v", input)
	if input.UserIDs != nil && len(input.UserIDs) > 0 {
		for _, userID := range input.UserIDs {
			user, err := userRepo.GetByID(c, userID)
			if err != nil {
				return err
			}
			err = opensea.UpdateAssetsForAcc(c, user.ID, user.Addresses, nftRepo, userRepo, collRepo)
			if err != nil {
				return err
			}
		}
	}
	if input.Addresses != nil && len(input.Addresses) > 0 {
		for _, address := range input.Addresses {
			if _, err := opensea.UpdateAssetsForWallet(c, []persist.Address{address}, nftRepo); err != nil {
				return err
			}
		}
	}
	return nil
}
