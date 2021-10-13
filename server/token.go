package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/persist/mongodb"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"google.golang.org/appengine"
)

type getTokensByIDInput struct {
	NftID persist.DBID `json:"id" form:"id" binding:"required"`
}

type getTokensByUserIDInput struct {
	UserID persist.DBID `json:"user_id" form:"user_id" binding:"required"`
	Page   int          `json:"page" form:"page"`
}

type getUnassignedTokensByUserIDInput struct {
	SkipCache bool `json:"skip_cache" form:"skip_cache"`
}

type getTokensOutput struct {
	Nfts []*persist.Token `json:"nfts"`
}

type getTokenByIDOutput struct {
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
	tokenID         string
	contractAddress string
}

type errCouldNotUpdateMedia struct {
	id persist.DBID
}

func getTokenByID(nftRepository persist.TokenRepository, ipfsClient *shell.Shell) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getTokensByIDInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		token, err := nftRepository.GetByID(c, input.NftID)
		if err != nil {
			status := http.StatusInternalServerError
			if _, ok := err.(persist.ErrTokenNotFoundByID); ok {
				status = http.StatusNotFound
			}
			util.ErrResponse(c, status, err)
			return
		}

		aeCtx := appengine.NewContext(c.Request)
		c.JSON(http.StatusOK, getTokenByIDOutput{Nft: ensureTokenMedia(aeCtx, []*persist.Token{token}, nftRepository, ipfsClient)[0]})
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

func getTokensForUser(nftRepository persist.TokenRepository, ipfsClient *shell.Shell) gin.HandlerFunc {
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

		c.JSON(http.StatusOK, getTokensOutput{Nfts: ensureTokenMedia(aeCtx, nfts, nftRepository, ipfsClient)})
	}
}

func getUnassignedTokensForUser(collectionRepository persist.CollectionTokenRepository, tokenRepository persist.TokenRepository, ipfsClient *shell.Shell) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &getUnassignedTokensByUserIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := getUserIDfromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}
		coll, err := collectionRepository.GetUnassigned(c, userID, input.SkipCache)
		if coll == nil || err != nil {
			coll = &persist.CollectionToken{Nfts: []*persist.TokenInCollection{}}
		}

		aeCtx := appengine.NewContext(c.Request)

		c.JSON(http.StatusOK, getUnassignedTokensOutput{Nfts: ensureCollectionTokenMedia(aeCtx, coll.Nfts, tokenRepository, ipfsClient)})
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

func ensureTokenMedia(aeCtx context.Context, nfts []*persist.Token, tokenRepo persist.TokenRepository, ipfsClient *shell.Shell) []*persist.Token {
	nftChan := make(chan *persist.Token)
	for _, nft := range nfts {
		go func(n *persist.Token) {
			if n.Media.MediaURL == "" {
				media, err := makePreviewsForMetadata(aeCtx, n.TokenMetadata, n.ContractAddress, n.TokenID, n.TokenURI, ipfsClient)
				if err == nil {
					n.Media = *media
					go func() {
						err := tokenRepo.UpdateByIDUnsafe(aeCtx, n.ID, persist.TokenUpdateMediaInput{Media: media})
						if err != nil {
							logrus.WithError(err).Error("could not update media for nft")
						}
					}()
				} else {
					logrus.WithError(err).Error("could not make media for nft")
				}
			}
			nftChan <- n
		}(nft)
	}
	for i := 0; i < len(nfts); i++ {
		nft := <-nftChan
		nfts[i] = nft
	}
	return nfts
}

func ensureCollectionTokenMedia(aeCtx context.Context, nfts []*persist.TokenInCollection, tokenRepo persist.TokenRepository, ipfsClient *shell.Shell) []*persist.TokenInCollection {
	nftChan := make(chan *persist.TokenInCollection)
	for _, nft := range nfts {
		go func(n *persist.TokenInCollection) {
			if n.Media.MediaURL == "" {
				media, err := makePreviewsForMetadata(aeCtx, n.TokenMetadata, n.ContractAddress, n.TokenID, n.TokenURI, ipfsClient)
				if err == nil {
					n.Media = *media
					go func() {
						err := tokenRepo.UpdateByIDUnsafe(aeCtx, n.ID, persist.TokenUpdateMediaInput{Media: media})
						if err != nil {
							logrus.WithError(err).Error(errCouldNotUpdateMedia{n.ID}.Error())
						}
					}()
				} else {
					logrus.WithError(err).Error(errCouldNotMakeMedia{n.TokenID, n.ContractAddress}.Error())
				}
			}
			nftChan <- n
		}(nft)
	}
	for i := 0; i < len(nfts); i++ {
		nft := <-nftChan
		nfts[i] = nft
	}
	return nfts
}

func (e errCouldNotMakeMedia) Error() string {
	return fmt.Sprintf("could not make media for token with address: %s at TokenID: %s", e.contractAddress, e.tokenID)
}

func (e errCouldNotUpdateMedia) Error() string {
	return fmt.Sprintf("could not update media for token with ID: %s", e.id)
}
