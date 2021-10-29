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
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/persist/mongodb"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"google.golang.org/appengine"
)

type getTokensInput struct {
	ID              persist.DBID    `form:"id"`
	WalletAddress   persist.Address `form:"address"`
	ContractAddress persist.Address `form:"contract_address"`
	TokenID         persist.TokenID `form:"token_id"`
}

type getTokensByUserIDInput struct {
	UserID persist.DBID `json:"user_id" form:"user_id" binding:"required"`
	Page   int          `json:"page" form:"page"`
}

type getTokensOutput struct {
	Nfts []*persist.Token `json:"nfts"`
}

type getTokenOutput struct {
	Nft *persist.Token `json:"nft"`
}

type getUnassignedTokensOutput struct {
	Nfts []*persist.TokenInCollection `json:"nfts"`
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
			util.ErrResponse(c, http.StatusBadRequest, errInvalidInput{reason: "must specify at least one of id, wallet_address, contract_address, token_id"})
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

		if token != nil {
			c.JSON(http.StatusOK, getTokenOutput{Nft: ensureTokenMedia(aeCtx, []*persist.Token{token}, nftRepository, ipfsClient, ethClient)[0]})
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
			c.JSON(http.StatusOK, getTokensOutput{Nfts: ensureTokenMedia(aeCtx, tokens, nftRepository, ipfsClient, ethClient)})
			return
		}

		c.JSON(http.StatusNotFound, util.ErrorResponse{Error: "no tokens found"})
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

		userID := getUserIDfromCtx(c)
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
		nfts, err := nftRepository.GetByUserID(c, input.UserID)
		if len(nfts) == 0 || err != nil {
			nfts = []*persist.Token{}
		}

		aeCtx := appengine.NewContext(c.Request)

		c.JSON(http.StatusOK, getTokensOutput{Nfts: ensureTokenMedia(aeCtx, nfts, nftRepository, ipfsClient, ethClient)})
	}
}

func getUnassignedTokensForUser(collectionRepository persist.CollectionTokenRepository, tokenRepository persist.TokenRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		userID := getUserIDfromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}
		coll, err := collectionRepository.GetUnassigned(c, userID)
		if coll == nil || err != nil {
			coll = &persist.CollectionToken{Nfts: []*persist.TokenInCollection{}}
		}

		aeCtx := appengine.NewContext(c.Request)

		c.JSON(http.StatusOK, getUnassignedTokensOutput{Nfts: ensureCollectionTokenMedia(aeCtx, coll.Nfts, tokenRepository, ipfsClient, ethClient)})
	}
}

func refreshUnassignedTokensForUser(collectionRepository persist.CollectionTokenRepository) gin.HandlerFunc {
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

func ensureTokenMedia(aeCtx context.Context, nfts []*persist.Token, tokenRepo persist.TokenRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client) []*persist.Token {
	nftChan := make(chan *persist.Token)
	for _, nft := range nfts {
		go func(n *persist.Token) {
			newMedia, newMetadata, newURI := ensureMetadataRelatedFields(aeCtx, n.ID, n.TokenType, n.Media, n.TokenMetadata, n.TokenURI, n.TokenID, n.ContractAddress, tokenRepo, ipfsClient, ethClient)
			n.Media = newMedia
			n.TokenMetadata = newMetadata
			n.TokenURI = newURI
			nftChan <- n
			go func(nm *persist.Token) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				err := tokenRepo.UpdateByIDUnsafe(ctx, nm.ID, persist.TokenUpdateMediaInput{Media: newMedia, Metadata: newMetadata})
				if err != nil {
					logrus.WithError(err).Error(errCouldNotUpdateMedia{nm.ID}.Error())
				}
			}(n)
		}(nft)
	}
	for i := 0; i < len(nfts); i++ {
		nft := <-nftChan
		nfts[i] = nft
	}
	return nfts
}

func ensureCollectionTokenMedia(aeCtx context.Context, nfts []*persist.TokenInCollection, tokenRepo persist.TokenRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client) []*persist.TokenInCollection {
	nftChan := make(chan *persist.TokenInCollection)
	for _, nft := range nfts {
		go func(n *persist.TokenInCollection) {
			newMedia, newMetadata, newURI := ensureMetadataRelatedFields(aeCtx, n.ID, n.TokenType, n.Media, n.TokenMetadata, n.TokenURI, n.TokenID, n.ContractAddress, tokenRepo, ipfsClient, ethClient)
			n.Media = newMedia
			n.TokenMetadata = newMetadata
			n.TokenURI = newURI
			nftChan <- n
			go func(nm *persist.TokenInCollection) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				err := tokenRepo.UpdateByIDUnsafe(ctx, nm.ID, persist.TokenUpdateMediaInput{Media: newMedia, Metadata: newMetadata})
				if err != nil {
					logrus.WithError(err).Error(errCouldNotUpdateMedia{nm.ID}.Error())
				}
			}(n)
		}(nft)
	}
	for i := 0; i < len(nfts); i++ {
		nft := <-nftChan
		nfts[i] = nft
	}
	return nfts
}

func ensureMetadataRelatedFields(ctx context.Context, id persist.DBID, tokenType persist.TokenType, media persist.Media, metadata persist.TokenMetadata, tokenURI persist.TokenURI, tokenID persist.TokenID, contractAddress persist.Address, tokenRepo persist.TokenRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client) (persist.Media, persist.TokenMetadata, persist.TokenURI) {
	if tokenURI == "" {
		if uri, err := indexer.GetTokenURI(ctx, tokenType, contractAddress, tokenID, ethClient); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{"contract": contractAddress, "tokenID": tokenID}).Error("could not get token URI for token")
		} else {
			tokenURI = uri
		}
	}
	if metadata == nil || len(metadata) == 0 {
		if m, err := indexer.GetMetadataFromURI(ctx, tokenURI, ipfsClient); err == nil {
			metadata = m
		}
	}
	if media.MediaURL == "" {
		newMedia, err := makePreviewsForMetadata(ctx, metadata, contractAddress, tokenID, tokenURI, ipfsClient)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{"contract": contractAddress, "tokenID": tokenID}).Error("could not make previews for token")
		} else {
			if newMedia.MediaURL == "" {
				if it, ok := util.GetValueFromMapUnsafe(metadata, "image", util.DefaultSearchDepth).(string); ok {
					newMedia.MediaURL = it
					newMedia.PreviewURL = it
					newMedia.ThumbnailURL = it
				}
			} else {
				media = *newMedia
			}
			if err != nil && newMedia.MediaURL != "" {
				logrus.WithError(err).Error(errCouldNotMakeMedia{tokenID, contractAddress}.Error())
			}
		}
	}
	return media, metadata, tokenURI
}

func getTokenFromDB(pCtx context.Context, input *getTokensInput, tokenRepo persist.TokenRepository) (*persist.Token, error) {
	switch {
	case input.ID != "":
		return tokenRepo.GetByID(pCtx, input.ID)
	}
	return nil, nil
}
func getTokensFromDB(pCtx context.Context, input *getTokensInput, tokenRepo persist.TokenRepository) ([]*persist.Token, error) {
	switch {
	case input.ID != "":
		token, err := tokenRepo.GetByID(pCtx, input.ID)
		if err != nil {
			return nil, err
		}
		return []*persist.Token{token}, nil
	case input.WalletAddress != "":
		return tokenRepo.GetByWallet(pCtx, input.WalletAddress)
	case input.TokenID != "" && input.ContractAddress != "":
		return tokenRepo.GetByTokenIdentifiers(pCtx, input.TokenID, input.ContractAddress)
	case input.ContractAddress != "":
		return tokenRepo.GetByContract(pCtx, input.ContractAddress)
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
