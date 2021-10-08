package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/copy"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

type getNftsByIDInput struct {
	NftID persist.DBID `json:"id" form:"id" binding:"required"`
}

type getNftsByUserIDInput struct {
	UserID persist.DBID `json:"user_id" form:"user_id" binding:"required"`
	Page   int          `json:"page" form:"page"`
}

type getUnassignedNFTByUserIDInput struct {
	SkipCache bool `json:"skip_cache" form:"skip_cache"`
}

type getOpenseaNftsInput struct {
	// Comma separated list of wallet addresses
	WalletAddresses string `json:"addresses" form:"addresses"`
	SkipCache       bool   `json:"skip_cache" form:"skip_cache"`
}

type syncBlockchainNftsInput struct {
	WalletAddresses string `json:"addresses" form:"addresses"`
	SkipDB          bool   `json:"skip_db" form:"skip_db"`
}

type getNftsOutput struct {
	Nfts []*persist.Token `json:"nfts"`
}

type getNftByIDOutput struct {
	Nft *persist.Token `json:"nft"`
}

type getUnassignedNftsOutput struct {
	Nfts []*persist.CollectionToken `json:"nfts"`
}

type updateNftByIDInput struct {
	ID             persist.DBID `json:"id" binding:"required"`
	CollectorsNote string       `json:"collectors_note" binding:"required"`
}

type getOwnershipHistoryInput struct {
	NftID     persist.DBID `json:"id" form:"id" binding:"required"`
	SkipCache bool         `json:"skip_cache" form:"skip_cache"`
}

type getOwnershipHistoryOutput struct {
	OwnershipHistory *persist.OwnershipHistory `json:"ownership_history"`
}

func getNftByID(nftRepository persist.TokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getNftsByIDInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{
				Error: copy.NftIDQueryNotProvided,
			})
			return
		}

		nfts, err := nftRepository.GetByID(c, input.NftID)
		if err != nil {
			logrus.WithError(err).Error("could not get nft by id")
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
			return
		}
		if len(nfts) == 0 {
			c.JSON(http.StatusNotFound, util.ErrorResponse{
				Error: fmt.Sprintf("no nfts found with id: %s", input.NftID),
			})
			return
		}

		if len(nfts) > 1 {
			nfts = nfts[:1]
			// TODO log that this should not be happening
		}
		c.JSON(http.StatusOK, getNftByIDOutput{Nft: nfts[0]})
	}
}

// Must specify nft id in json input
func updateNftByID(nftRepository persist.TokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &updateNftByIDInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		userID := getUserIDfromCtx(c)
		if userID == "" {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: "user id not found in context"})
			return
		}

		update := &persist.TokenUpdateInfoInput{CollectorsNote: input.CollectorsNote}

		err := nftRepository.UpdateByID(c, input.ID, userID, update)
		if err != nil {
			if err.Error() == copy.CouldNotFindDocument {
				c.JSON(http.StatusNotFound, util.ErrorResponse{Error: err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func getNftsForUser(nftRepository persist.TokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getNftsByUserIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: err.Error()})
			return
		}
		nfts, err := nftRepository.GetByUserID(c, input.UserID)
		if len(nfts) == 0 || err != nil {
			nfts = []*persist.Token{}
		}

		c.JSON(http.StatusOK, getNftsOutput{Nfts: nfts})
	}
}

func getUnassignedNftsForUser(collectionRepository persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getUnassignedNFTByUserIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: err.Error()})
			return
		}

		userID := getUserIDfromCtx(c)
		if userID == "" {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: "user id not found in context"})
			return
		}
		coll, err := collectionRepository.GetUnassigned(c, userID, input.SkipCache)
		if coll == nil || err != nil {
			coll = &persist.Collection{Nfts: []*persist.CollectionToken{}}
		}

		c.JSON(http.StatusOK, getUnassignedNftsOutput{Nfts: coll.Nfts})
	}
}

func doesUserOwnWallets(pCtx context.Context, userID persist.DBID, walletAddresses []string, userRepo persist.UserRepository) (bool, error) {
	user, err := userRepo.GetByID(pCtx, userID)
	if err != nil {
		return false, err
	}
	for _, walletAddress := range walletAddresses {
		if !util.Contains(user.Addresses, walletAddress) {
			return false, nil
		}
	}
	return true, nil
}
