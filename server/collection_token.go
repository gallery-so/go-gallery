package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"

	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

var errTooManyTokensInCollection = errors.New("a collection can have no more than 1000 tokens")

type collectionGetByUserIDInputToken struct {
	UserID persist.DBID `form:"user_id" json:"user_id" binding:"required"`
}
type collectionGetByIDInputToken struct {
	ID persist.DBID `form:"id" json:"id" binding:"required"`
}
type collectionGetByIDOutputToken struct {
	Collection persist.CollectionToken `json:"collection"`
}

type collectionGetOutputtoken struct {
	Collections []persist.CollectionToken `json:"collections"`
}

type collectionCreateInputToken struct {
	GalleryID      persist.DBID        `json:"gallery_id" binding:"required"`
	Nfts           []persist.DBID      `json:"nfts" binding:"required"`
	Layout         persist.TokenLayout `json:"layout" `
	Name           string              `json:"name"`
	CollectorsNote string              `json:"collectors_note" binding:"medium"`
}

type collectionUpdateInfoByIDInputToken struct {
	ID             persist.DBID `json:"id" binding:"required"`
	Name           string       `json:"name"`
	CollectorsNote string       `json:"collectors_note" binding:"medium"`
}

type collectionUpdateHiddenByIDInputToken struct {
	ID     persist.DBID `json:"id" binding:"required"`
	Hidden bool         `json:"hidden"`
}
type collectionUpdateNftsByIDInputToken struct {
	ID     persist.DBID        `json:"id" binding:"required"`
	Nfts   []persist.DBID      `json:"nfts" binding:"required"`
	Layout persist.TokenLayout `json:"layout"`
}

type collectionCreateOutputToken struct {
	ID persist.DBID `json:"collection_id"`
}

type collectionDeleteInputToken struct {
	ID persist.DBID `json:"id" binding:"required"`
}

type errNoCollectionsFoundWithID struct {
	id persist.DBID
}

// HANDLERS

func getCollectionsByUserIDToken(collectionsRepository persist.CollectionTokenRepository, tokenRepository persist.TokenRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &collectionGetByUserIDInputToken{}
		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		colls, err := collectionsRepository.GetByUserID(c, input.UserID)
		if len(colls) == 0 || err != nil {
			colls = []persist.CollectionToken{}
		}

		c.JSON(http.StatusOK, collectionGetOutputtoken{Collections: colls})

	}
}

func getCollectionByIDToken(collectionsRepository persist.CollectionTokenRepository, tokenRepository persist.TokenRepository, ipfsClient *shell.Shell, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &collectionGetByIDInputToken{}
		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		coll, err := collectionsRepository.GetByID(c, input.ID)
		if err != nil {
			status := http.StatusInternalServerError
			if _, ok := err.(persist.ErrCollectionNotFoundByID); ok {
				status = http.StatusNotFound
			}
			util.ErrResponse(c, status, err)
			return
		}

		c.JSON(http.StatusOK, collectionGetByIDOutputToken{Collection: coll})
	}
}

func createCollectionToken(collectionsRepository persist.CollectionTokenRepository, galleryRepository persist.GalleryTokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := collectionCreateInputToken{}
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := auth.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		//------------------
		// CREATE

		id, err := collectionCreateDbToken(c, input, userID, collectionsRepository, galleryRepository)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, collectionCreateOutputToken{ID: id})
	}
}

func updateCollectionInfoToken(collectionsRepository persist.CollectionTokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateInfoByIDInputToken{}
		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := auth.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		update := persist.CollectionTokenUpdateInfoInput{Name: persist.NullString(validate.SanitizationPolicy.Sanitize(input.Name)), CollectorsNote: persist.NullString(validate.SanitizationPolicy.Sanitize(input.CollectorsNote))}

		err := collectionsRepository.Update(c, input.ID, userID, update)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func updateCollectionHiddenToken(collectionsRepository persist.CollectionTokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateHiddenByIDInputToken{}
		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := auth.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		update := persist.CollectionTokenUpdateHiddenInput{Hidden: persist.NullBool(input.Hidden)}

		err := collectionsRepository.Update(c, input.ID, userID, update)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func updateCollectionTokensToken(collectionsRepository persist.CollectionTokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateNftsByIDInputToken{}
		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		// TODO magic number
		if len(input.Nfts) > 1000 {
			util.ErrResponse(c, http.StatusBadRequest, errTooManyTokensInCollection)
			return
		}

		userID := auth.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		// ensure that there are no repeat NFTs
		withNoRepeats := uniqueDBID(input.Nfts)

		layout, err := persist.ValidateLayout(input.Layout, input.Nfts)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		update := persist.CollectionTokenUpdateNftsInput{NFTs: withNoRepeats, Layout: layout}

		err = collectionsRepository.UpdateNFTs(c, input.ID, userID, update)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func deleteCollectionToken(collectionsRepository persist.CollectionTokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionDeleteInputToken{}
		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := auth.GetUserIDFromCtx(c)
		if userID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errUserIDNotInCtx)
			return
		}

		err := collectionsRepository.Delete(c, input.ID, userID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

// CREATE
func collectionCreateDbToken(pCtx context.Context, pInput collectionCreateInputToken, pUserID persist.DBID, collectionsRepo persist.CollectionTokenRepository, galleryRepo persist.GalleryTokenRepository) (persist.DBID, error) {

	layout, err := persist.ValidateLayout(pInput.Layout, pInput.Nfts)
	if err != nil {
		return "", err
	}
	coll := persist.CollectionTokenDB{
		OwnerUserID:    pUserID,
		NFTs:           pInput.Nfts,
		Name:           persist.NullString(validate.SanitizationPolicy.Sanitize(pInput.Name)),
		Layout:         layout,
		CollectorsNote: persist.NullString(validate.SanitizationPolicy.Sanitize(pInput.CollectorsNote)),
	}

	collID, err := collectionsRepo.Create(pCtx, coll)
	if err != nil {
		return "", err
	}

	err = galleryRepo.AddCollections(pCtx, pInput.GalleryID, pUserID, []persist.DBID{collID})
	if err != nil {
		return "", err
	}

	return collID, nil

}

// uniqueDBID ensures that an array of DBIDs has no repeat items
func uniqueDBID(a []persist.DBID) []persist.DBID {
	result := make([]persist.DBID, 0, len(a))
	m := map[persist.DBID]bool{}

	for _, val := range a {
		if _, ok := m[val]; !ok {
			m[val] = true
			result = append(result, val)
		}
	}

	return result
}

func (e errNoCollectionsFoundWithID) Error() string {
	return fmt.Sprintf("no collections found with ID %s", e.id)
}
