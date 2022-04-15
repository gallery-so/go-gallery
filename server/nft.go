package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

type getNftsByIDInput struct {
	NftID persist.DBID `json:"id" form:"id" binding:"required"`
}

type getNftsByUserIDInput struct {
	UserID persist.DBID `json:"user_id" form:"user_id" binding:"required"`
}

type getOpenseaNftsInput struct {
	// Comma separated list of wallet addresses
	WalletAddresses string `json:"addresses" form:"addresses"`
}
type refreshOpenseaNftsInput struct {
	// Comma separated list of wallet addresses
	WalletAddresses string `json:"addresses" form:"addresses"`
}

type getNftsOutput struct {
	Nfts []persist.NFT `json:"nfts"`
}

type getNftByIDOutput struct {
	Nft persist.NFT `json:"nft"`
}

type getUnassignedNftsOutput struct {
	Nfts []persist.CollectionNFT `json:"nfts"`
}

type updateNftByIDInput struct {
	ID             persist.DBID `json:"id" binding:"required"`
	CollectorsNote string       `json:"collectors_note" binding:"collectors_note"`
}

func getNftByID(nftRepository persist.NFTRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getNftsByIDInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		nft, err := nftRepository.GetByID(c, input.NftID)
		if err != nil {
			status := http.StatusInternalServerError
			if _, ok := err.(persist.ErrNFTNotFoundByID); ok {
				status = http.StatusNotFound
			}
			util.ErrResponse(c, status, err)
			return
		}

		c.JSON(http.StatusOK, getNftByIDOutput{Nft: nft})
	}
}

// Must specify nft id in json input
func updateNftByID(nftRepository persist.NFTRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &updateNftByIDInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := auth.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		update := persist.NFTUpdateInfoInput{CollectorsNote: persist.NullString(validate.SanitizationPolicy.Sanitize(input.CollectorsNote))}

		err := nftRepository.UpdateByID(c, input.ID, userID, update)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func getNftsForUser(nftRepository persist.NFTRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getTokensByUserIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		nfts, err := nftRepository.GetByUserID(c, input.UserID)
		if nfts == nil || err != nil {
			nfts = []persist.NFT{}
		}

		c.JSON(http.StatusOK, getNftsOutput{Nfts: nfts})
	}
}
