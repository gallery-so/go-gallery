package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

type getTokensInput struct {
	ID              persist.DBID    `form:"id"`
	WalletAddress   persist.Address `form:"address"`
	ContractAddress persist.Address `form:"contract_address"`
	TokenID         persist.TokenID `form:"token_id"`
	Page            int64           `form:"page"`
	Limit           int64           `form:"limit"`
}

type getTokensByUserIDInput struct {
	UserID persist.DBID `json:"user_id" form:"user_id" binding:"required"`
	Page   int64        `json:"page" form:"page"`
	Limit  int64        `json:"limit" form:"limit"`
}

type getTokensOutput struct {
	Nfts []persist.Token `json:"nfts"`
}

type getTokenOutput struct {
	Nft persist.Token `json:"nft"`
}

type getUnassignedTokensOutput struct {
	Nfts []persist.TokenInCollection `json:"nfts"`
}

type updateTokenByIDInput struct {
	ID             persist.DBID `json:"id" binding:"required"`
	CollectorsNote string       `json:"collectors_note" binding:"required,collectors_note"`
}

type errCouldNotMakeMedia struct {
	tokenID         persist.TokenID
	contractAddress persist.Address
}

type errCouldNotUpdateMedia struct {
	id persist.DBID
}

type errInvalidInput struct {
	reason string
}

func getTokens(nftRepository persist.TokenRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getTokensInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		if input.ID == "" && input.WalletAddress == "" && input.ContractAddress == "" && input.TokenID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errInvalidInput{reason: "must specify at least one of id, address, contract_address, token_id"})
			return
		}

		token, err := getTokenFromDB(c, input, nftRepository)
		if err != nil {
			status := http.StatusInternalServerError
			if _, ok := err.(persist.ErrTokenNotFoundByID); ok {
				status = http.StatusNotFound
			}
			util.ErrResponse(c, status, err)
			return
		}

		if token.ID != "" {

			c.JSON(http.StatusOK, getTokenOutput{Nft: token})
			return
		}
		tokens, err := getTokensFromDB(c, input, nftRepository)
		if err != nil {
			status := http.StatusInternalServerError
			if _, ok := err.(persist.ErrTokenNotFoundByIdentifiers); ok {
				status = http.StatusNotFound
			}
			util.ErrResponse(c, status, err)
			return
		}
		if tokens != nil {

			c.JSON(http.StatusOK, getTokensOutput{Nfts: tokens})
			return
		}

		util.ErrResponse(c, http.StatusInternalServerError, fmt.Errorf("no tokens found"))
	}
}

// Must specify nft id in json input
func updateTokenByID(nftRepository persist.TokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &updateTokenByIDInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := auth.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		update := persist.TokenUpdateInfoInput{CollectorsNote: persist.NullString(input.CollectorsNote)}

		err := nftRepository.UpdateByID(c, input.ID, userID, update)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func getTokensForUser(nftRepository persist.TokenRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getTokensByUserIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		nfts, err := nftRepository.GetByUserID(c, input.UserID, input.Limit, input.Page)
		if nfts == nil || err != nil {
			nfts = []persist.Token{}
		}

		c.JSON(http.StatusOK, getTokensOutput{Nfts: nfts})
	}
}

func getUnassignedTokensForUser(collectionRepository persist.CollectionTokenRepository, tokenRepository persist.TokenRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		userID := auth.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}
		coll, err := collectionRepository.GetUnassigned(c, userID)
		if err != nil {
			coll.NFTs = []persist.TokenInCollection{}
		}

		c.JSON(http.StatusOK, getUnassignedTokensOutput{Nfts: coll.NFTs})
	}
}

func refreshUnassignedTokensForUser(collectionRepository persist.CollectionTokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		userID := auth.GetUserIDFromCtx(c)
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

func doesUserOwnWallets(pCtx context.Context, userID persist.DBID, walletAddresses []persist.Address, userRepo persist.UserRepository) (bool, error) {
	user, err := userRepo.GetByID(pCtx, userID)
	if err != nil {
		return false, err
	}
	for _, walletAddress := range walletAddresses {
		if !containsWalletAddresses(user.Addresses, walletAddress) {
			return false, nil
		}
	}
	return true, nil
}

func getTokenFromDB(pCtx context.Context, input *getTokensInput, tokenRepo persist.TokenRepository) (persist.Token, error) {
	switch {
	case input.ID != "":
		return tokenRepo.GetByID(pCtx, input.ID)
	}
	return persist.Token{}, nil
}
func getTokensFromDB(pCtx context.Context, input *getTokensInput, tokenRepo persist.TokenRepository) ([]persist.Token, error) {
	switch {
	case input.ID != "":
		token, err := tokenRepo.GetByID(pCtx, input.ID)
		if err != nil {
			return nil, err
		}
		return []persist.Token{token}, nil
	case input.WalletAddress != "":
		return tokenRepo.GetByWallet(pCtx, input.WalletAddress, input.Limit, input.Page)
	case input.TokenID != "" && input.ContractAddress != "":
		if strings.HasPrefix(string(input.TokenID), "0x") {
			input.TokenID = input.TokenID[2:]
		} else {
			input.TokenID = persist.TokenID(input.TokenID.BigInt().Text(16))
		}

		return tokenRepo.GetByTokenIdentifiers(pCtx, input.TokenID, input.ContractAddress, input.Limit, input.Page)
	case input.ContractAddress != "":
		return tokenRepo.GetByContract(pCtx, input.ContractAddress, input.Limit, input.Page)
	}
	return nil, nil

}

func (e errCouldNotMakeMedia) Error() string {
	return fmt.Sprintf("could not make media for token with address: %s at TokenID: %s", e.contractAddress, e.tokenID)
}

func (e errCouldNotUpdateMedia) Error() string {
	return fmt.Sprintf("could not update media for token with ID: %s", e.id)
}

func (e errInvalidInput) Error() string {
	return fmt.Sprintf("invalid input: %s", e.reason)
}

func containsWalletAddresses(a []persist.Address, b persist.Address) bool {
	for _, v := range a {
		if v == b {
			return true
		}
	}

	return false
}
