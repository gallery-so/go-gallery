package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

var errNoTokensToRefresh = errors.New("no tokens to refresh metadata for")

type getTokensInput struct {
	ID              persist.DBID    `form:"id"`
	WalletAddress   persist.Wallet  `form:"address"`
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
	Nfts []persist.TokenGallery `json:"nfts"`
}

type getTokenOutput struct {
	Nft persist.TokenGallery `json:"nft"`
}

type getUnassignedTokensOutput struct {
	Nfts []persist.TokenInCollection `json:"nfts"`
}

type updateTokenByIDInput struct {
	ID             persist.DBID `json:"id" binding:"required"`
	CollectorsNote string       `json:"collectors_note" binding:"required,collectors_note"`
}

type refreshMetadataInput struct {
	TokenID         persist.TokenID `form:"token_id,required"`
	ContractAddress persist.Address `form:"contract_address,required"`
}

type refreshMetadataOutput struct {
	Token persist.TokenGallery `json:"token"`
}
type errCouldNotMakeMedia struct {
	tokenID         persist.TokenID
	contractAddress persist.Address
}

type errCouldNotUpdateMedia struct {
	id persist.DBID
}

func getTokens(nftRepository persist.TokenGalleryRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getTokensInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		if input.ID == "" && input.WalletAddress.String() == "" && input.ContractAddress == "" && input.TokenID == "" {
			util.ErrResponse(c, http.StatusBadRequest, util.ErrInvalidInput{Reason: "must specify at least one of id, address, contract_address, token_id"})
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
func updateTokenByID(nftRepository persist.TokenGalleryRepository) gin.HandlerFunc {
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

func getTokensForUser(nftRepository persist.TokenGalleryRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getTokensByUserIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		nfts, err := nftRepository.GetByUserID(c, input.UserID, input.Limit, input.Page)
		if nfts == nil || err != nil {
			nfts = []persist.TokenGallery{}
		}

		c.JSON(http.StatusOK, getTokensOutput{Nfts: nfts})
	}
}

func getUnassignedTokensForUser(collectionRepository persist.CollectionTokenRepository, tokenRepository persist.TokenGalleryRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client) gin.HandlerFunc {
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

// TODO use multichain handlers
func refreshMetadataForToken(tokenRepository persist.TokenGalleryRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input refreshMetadataInput
		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		tokens, err := tokenRepository.GetByTokenIdentifiers(c, input.TokenID, input.ContractAddress, -1, -1)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if len(tokens) == 0 {
			util.ErrResponse(c, http.StatusBadRequest, errNoTokensToRefresh)
			return
		}
		var result refreshMetadataOutput
		for _, t := range tokens {
			token, err := refreshMetadata(c, t, ethcl, ipfsClient, arweaveClient, storageClient)
			if err != nil {
				logrus.WithError(err).Error("could not refresh all metadata for token")
			}
			update := persist.TokenUpdateMediaInput{
				TokenURI: token.TokenURI,
				Metadata: token.TokenMetadata,
				Media:    token.Media,
			}

			if err := tokenRepository.UpdateByIDUnsafe(c, token.ID, update); err != nil {
				util.ErrResponse(c, http.StatusInternalServerError, fmt.Errorf("failed to update token %s: %v", token.TokenID, err))
			}
			if result.Token.ID == "" {
				result.Token = token
			}
		}

		c.JSON(http.StatusOK, result)
		return
	}
}

func getTokenFromDB(pCtx context.Context, input *getTokensInput, tokenRepo persist.TokenGalleryRepository) (persist.TokenGallery, error) {
	switch {
	case input.ID != "":
		return tokenRepo.GetByID(pCtx, input.ID)
	}
	return persist.TokenGallery{}, nil
}
func getTokensFromDB(pCtx context.Context, input *getTokensInput, tokenRepo persist.TokenGalleryRepository) ([]persist.TokenGallery, error) {
	switch {
	case input.ID != "":
		token, err := tokenRepo.GetByID(pCtx, input.ID)
		if err != nil {
			return nil, err
		}
		return []persist.TokenGallery{token}, nil
	case input.WalletAddress.String() != "":
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

func refreshMetadata(ctx context.Context, token persist.TokenGallery, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) (persist.TokenGallery, error) {

	uri, err := rpc.GetTokenURI(ctx, token.TokenType, persist.EthereumAddress(token.ContractAddress), token.TokenID, ethcl)
	if err != nil {
		return token, fmt.Errorf("failed to get token URI for token %s-%s: %v", token.ContractAddress, token.TokenID, err)
	}

	uri = uri.ReplaceID(token.TokenID)

	token.TokenURI = uri

	metadata, err := rpc.GetMetadataFromURI(ctx, token.TokenURI, ipfsClient, arweaveClient)
	if err != nil {
		return token, fmt.Errorf("failed to get token metadata for token %s-%s with uri %s: %v", token.ContractAddress, token.TokenID, token.TokenURI, err)
	}
	token.TokenMetadata = metadata

	med, err := media.MakePreviewsForMetadata(ctx, metadata, token.ContractAddress, token.TokenID, uri, ipfsClient, arweaveClient, storageClient)
	if err != nil {
		return token, fmt.Errorf("failed to get token media for token %s-%s with uri %s: %v", token.ContractAddress, token.TokenID, token.TokenURI, err)
	}

	token.Media = med

	return token, nil

}

func (e errCouldNotMakeMedia) Error() string {
	return fmt.Sprintf("could not make media for token with address: %s at TokenID: %s", e.contractAddress, e.tokenID)
}

func (e errCouldNotUpdateMedia) Error() string {
	return fmt.Sprintf("could not update media for token with ID: %s", e.id)
}
