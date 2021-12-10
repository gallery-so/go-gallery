package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/indexer"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/mongodb"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"google.golang.org/appengine"
)

type getTokensInput struct {
	ID              persist.DBID    `form:"id"`
	WalletAddress   persist.Address `form:"address"`
	ContractAddress persist.Address `form:"contract_address"`
	TokenID         persist.TokenID `form:"token_id"`
	Page            int64           `form:"page"`
	Limit           int64           `form:"limit"`
	SkipMedia       bool            `form:"skip_media"`
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
	CollectorsNote string       `json:"collectors_note" binding:"required"`
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

		aeCtx := appengine.NewContext(c.Request)

		if token.ID != "" {
			if !input.SkipMedia {
				token = ensureTokenMedia(aeCtx, []persist.Token{token}, nftRepository, ipfsClient, ethClient)[0]
			}
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
			if !input.SkipMedia {
				tokens = ensureTokenMedia(aeCtx, tokens, nftRepository, ipfsClient, ethClient)
			}
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

		userID := middleware.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		update := &persist.TokenUpdateInfoInput{CollectorsNote: input.CollectorsNote}

		err := nftRepository.UpdateByID(c, input.ID, userID, update)
		if err != nil {
			if err == mongodb.ErrDocumentNotFound {
				c.JSON(http.StatusNotFound, util.ErrorResponse{Error: err.Error()})
				return
			}
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

		aeCtx := appengine.NewContext(c.Request)

		c.JSON(http.StatusOK, getTokensOutput{Nfts: ensureTokenMedia(aeCtx, nfts, nftRepository, ipfsClient, ethClient)})
	}
}

func getUnassignedTokensForUser(collectionRepository persist.CollectionTokenRepository, tokenRepository persist.TokenRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		userID := middleware.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}
		coll, err := collectionRepository.GetUnassigned(c, userID)
		if err != nil {
			coll.Nfts = []persist.TokenInCollection{}
		}

		aeCtx := appengine.NewContext(c.Request)

		c.JSON(http.StatusOK, getUnassignedTokensOutput{Nfts: ensureCollectionTokenMedia(aeCtx, coll.Nfts, tokenRepository, ipfsClient, ethClient)})
	}
}

func refreshUnassignedTokensForUser(collectionRepository persist.CollectionTokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		userID := middleware.GetUserIDFromCtx(c)
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

type tokenIndexTuple struct {
	token persist.Token
	i     int
}

func ensureTokenMedia(aeCtx context.Context, nfts []persist.Token, tokenRepo persist.TokenRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client) []persist.Token {
	nftChan := make(chan tokenIndexTuple)
	for i, nft := range nfts {
		go func(index int, n persist.Token) {
			newMedia, newMetadata, newURI := ensureMetadataRelatedFields(aeCtx, n.ID, n.TokenType, n.Media, n.TokenMetadata, n.TokenURI, n.TokenID, n.ContractAddress, tokenRepo, ipfsClient, ethClient)
			n.Media = newMedia
			n.TokenMetadata = newMetadata
			n.TokenURI = newURI
			nftChan <- tokenIndexTuple{n, index}
			go func(id persist.DBID) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				err := tokenRepo.UpdateByIDUnsafe(ctx, id, persist.TokenUpdateMediaInput{Media: newMedia, Metadata: newMetadata})
				if err != nil {
					logrus.WithError(err).Error(errCouldNotUpdateMedia{id}.Error())
				}
			}(n.ID)
		}(i, nft)
	}
	for i := 0; i < len(nfts); i++ {
		nft := <-nftChan
		nfts[nft.i] = nft.token
	}
	return nfts
}

type tokenCollectionIndexTuple struct {
	token persist.TokenInCollection
	i     int
}

func ensureCollectionTokenMedia(aeCtx context.Context, nfts []persist.TokenInCollection, tokenRepo persist.TokenRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client) []persist.TokenInCollection {

	nftChan := make(chan tokenCollectionIndexTuple)
	for i, nft := range nfts {
		go func(index int, n persist.TokenInCollection) {
			newMedia, newMetadata, newURI := ensureMetadataRelatedFields(aeCtx, n.ID, n.TokenType, n.Media, n.TokenMetadata, n.TokenURI, n.TokenID, n.ContractAddress, tokenRepo, ipfsClient, ethClient)
			n.Media = newMedia
			n.TokenMetadata = newMetadata
			n.TokenURI = newURI
			nftChan <- tokenCollectionIndexTuple{n, index}
			go func(id persist.DBID) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				err := tokenRepo.UpdateByIDUnsafe(ctx, id, persist.TokenUpdateMediaInput{Media: newMedia, Metadata: newMetadata})
				if err != nil {
					logrus.WithError(err).Error(errCouldNotUpdateMedia{id}.Error())
				}
			}(n.ID)
		}(i, nft)
	}
	for i := 0; i < len(nfts); i++ {
		nft := <-nftChan
		nfts[nft.i] = nft.token
	}
	return nfts
}

func ensureMetadataRelatedFields(ctx context.Context, id persist.DBID, tokenType persist.TokenType, media persist.Media, metadata persist.TokenMetadata, tokenURI persist.TokenURI, tokenID persist.TokenID, contractAddress persist.Address, tokenRepo persist.TokenRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client) (persist.Media, persist.TokenMetadata, persist.TokenURI) {
	if tokenURI == "" {
		logrus.Infof("Token URI is empty for token %s-%s", contractAddress, id)
		uri, err := indexer.GetTokenURI(ctx, tokenType, contractAddress, tokenID, ethClient)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{"contract": contractAddress, "tokenID": tokenID}).Error("could not get token URI for token")
			return media, metadata, tokenURI
		}
		tokenURI = uri

	}
	if metadata == nil || len(metadata) == 0 {
		logrus.Infof("Token metadata is empty for token %s-%s", contractAddress, id)
		m, err := indexer.GetMetadataFromURI(tokenURI, ipfsClient)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{"uri": tokenURI}).Error("could not get metadata for token")
			return media, metadata, tokenURI
		}
		metadata = m
	}
	if media.MediaType == "" {
		logrus.Infof("Token media type is empty for token %s-%s", contractAddress, id)
		newMedia, err := makePreviewsForMetadata(ctx, metadata, contractAddress, tokenID, tokenURI, ipfsClient)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{"contract": contractAddress, "tokenID": tokenID}).Error("could not make previews for token")
			return media, metadata, tokenURI
		}
		media = *newMedia
	}
	return media, metadata, tokenURI
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
