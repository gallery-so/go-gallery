package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/mongodb"
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
	ID             persist.DBID `form:"id" binding:"required"`
	CollectorsNote string       `form:"collectors_note"`
}

type getOwnershipHistoryInput struct {
	NftID     persist.DBID `json:"id" form:"id" binding:"required"`
	SkipCache bool         `json:"skip_cache" form:"skip_cache"`
}

type getOwnershipHistoryOutput struct {
	OwnershipHistory persist.OwnershipHistory `json:"ownership_history"`
}

type errDoesNotOwnWallets struct {
	id        persist.DBID
	addresses []persist.Address
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

		userID := middleware.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		update := persist.NFTUpdateInfoInput{CollectorsNote: persist.NullString(validate.SanitizationPolicy.Sanitize(input.CollectorsNote))}

		err := nftRepository.UpdateByID(c, input.ID, userID, update)
		if err != nil {
			status := http.StatusInternalServerError
			if err == mongodb.ErrDocumentNotFound {
				status = http.StatusNotFound
			}
			util.ErrResponse(c, status, err)
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

func getUnassignedNftsForUser(collectionRepository persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		userID := middleware.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}
		coll, err := collectionRepository.GetUnassigned(c, userID)
		if err != nil {
			coll = persist.Collection{NFTs: []persist.CollectionNFT{}}
		}

		c.JSON(http.StatusOK, getUnassignedNftsOutput{Nfts: coll.NFTs})
	}
}

func refreshUnassignedNftsForUser(collectionRepository persist.CollectionRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		userID := middleware.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}
		if err := collectionRepository.RefreshUnassigned(c, userID); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func getNftsFromOpensea(nftRepo persist.NFTRepository, userRepo persist.UserRepository, collRepo persist.CollectionRepository, historyRepo persist.OwnershipHistoryRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getOpenseaNftsInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := middleware.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		addresses := []persist.Address{}
		if input.WalletAddresses != "" {
			addresses = []persist.Address{persist.Address(input.WalletAddresses)}
			if strings.Contains(input.WalletAddresses, ",") {
				addressesStrings := strings.Split(input.WalletAddresses, ",")
				for _, address := range addressesStrings {
					addresses = append(addresses, persist.Address(address))
				}
			}
			ownsWallet, err := doesUserOwnWallets(c, userID, addresses, userRepo)
			if err != nil {
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}
			if !ownsWallet {
				util.ErrResponse(c, http.StatusBadRequest, errDoesNotOwnWallets{userID, addresses})
				return
			}
		}

		err := opensea.UpdateAssetsForAcc(c, userID, addresses, nftRepo, userRepo, collRepo, historyRepo)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}
func refreshOpenseaNFTs(nftRepo persist.NFTRepository, userRepo persist.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &refreshOpenseaNftsInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := middleware.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		addresses := []persist.Address{}
		if input.WalletAddresses != "" {
			addresses = []persist.Address{persist.Address(input.WalletAddresses)}
			if strings.Contains(input.WalletAddresses, ",") {
				addressesStrings := strings.Split(input.WalletAddresses, ",")
				for _, address := range addressesStrings {
					addresses = append(addresses, persist.Address(address))
				}
			}
			ownsWallet, err := doesUserOwnWallets(c, userID, addresses, userRepo)
			if err != nil {
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}
			if !ownsWallet {
				util.ErrResponse(c, http.StatusBadRequest, errDoesNotOwnWallets{id: userID, addresses: addresses})
				return
			}
		}

		if err := nftRepo.OpenseaCacheDelete(c, addresses); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func (e errDoesNotOwnWallets) Error() string {
	return fmt.Sprintf("user with ID %s does not own all wallets: %+v", e.id, e.addresses)
}
