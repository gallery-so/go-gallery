package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/copy"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
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
	SkipCache       bool   `json:"skip_cache" form:"skip_cache"`
}

type getNftsOutput struct {
	Nfts []*persist.NFT `json:"nfts"`
}

type getNftByIDOutput struct {
	Nft *persist.NFT `json:"nft"`
}

type getUnassignedNftsOutput struct {
	Nfts []*persist.CollectionNFT `json:"nfts"`
}

type updateNftByIDInput struct {
	ID             persist.DBID `form:"id" binding:"required"`
	CollectorsNote string       `form:"collectors_note"`
}

type getOwnershipHistoryInput struct {
	NftID     persist.DBID `json:"id" form:"id" binding:"required"`
	SkipCache bool         `json:"skip_cache" form:"skip_cache"`
}

type getOwnershipHistoryOutput struct {
	OwnershipHistory *persist.OwnershipHistory `json:"ownership_history"`
}

type errNoNFTsFoundWithID struct {
	id persist.DBID
}

type errDoesNotOwnWallets struct {
	id        persist.DBID
	addresses []string
}

func getNftByID(nftRepository persist.NFTRepository) gin.HandlerFunc {
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
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
			return
		}
		if len(nfts) == 0 {
			c.JSON(http.StatusNotFound, util.ErrorResponse{
				Error: errNoNFTsFoundWithID{id: input.NftID}.Error(),
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
func updateNftByID(nftRepository persist.NFTRepository) gin.HandlerFunc {
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
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: errUserIDNotInCtx.Error()})
			return
		}

		update := &persist.UpdateNFTInfoInput{CollectorsNote: sanitizationPolicy.Sanitize(input.CollectorsNote)}

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

func getNftsForUser(nftRepository persist.NFTRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getTokensByUserIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: err.Error()})
			return
		}
		nfts, err := nftRepository.GetByUserID(c, input.UserID)
		if len(nfts) == 0 || err != nil {
			nfts = []*persist.NFT{}
		}

		c.JSON(http.StatusOK, getNftsOutput{Nfts: nfts})
	}
}

func getUnassignedNftsForUser(collectionRepository persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		userID := getUserIDfromCtx(c)
		if userID == "" {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: errUserIDNotInCtx.Error()})
			return
		}
		coll, err := collectionRepository.GetUnassigned(c, userID)
		if coll == nil || err != nil {
			coll = &persist.Collection{Nfts: []*persist.CollectionNFT{}}
		}

		c.JSON(http.StatusOK, getUnassignedNftsOutput{Nfts: coll.Nfts})
	}
}

func refreshUnassignedNftsForUser(collectionRepository persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		userID := getUserIDfromCtx(c)
		if userID == "" {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: errUserIDNotInCtx.Error()})
			return
		}
		if err := collectionRepository.RefreshUnassigned(c, userID); err != nil {
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func getNftsFromOpensea(nftRepo persist.NFTRepository, userRepo persist.UserRepository, collRepo persist.CollectionRepository, historyRepo persist.OwnershipHistoryRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getOpenseaNftsInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: err.Error()})
			return
		}

		userID := getUserIDfromCtx(c)
		if userID == "" {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: errUserIDNotInCtx.Error()})
			return
		}

		addresses := strings.Split(input.WalletAddresses, ",")
		if len(addresses) > 0 {
			ownsWallet, err := doesUserOwnWallets(c, userID, addresses, userRepo)
			if err != nil {
				c.JSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
				return
			}
			if !ownsWallet {
				c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: errDoesNotOwnWallets{userID, addresses}.Error()})
				return
			}
		}

		nfts, err := openSeaPipelineAssetsForAcc(c, userID, addresses, input.SkipCache, nftRepo, userRepo, collRepo, historyRepo)
		if len(nfts) == 0 || err != nil {
			nfts = []*persist.NFT{}
		}

		c.JSON(http.StatusOK, getNftsOutput{Nfts: nfts})
	}
}

func (e errNoNFTsFoundWithID) Error() string {
	return fmt.Sprintf("no nfts found with id: %s", e.id)
}

func (e errDoesNotOwnWallets) Error() string {
	return fmt.Sprintf("user with ID %s does not own all wallets: %+v", e.id, e.addresses)
}
