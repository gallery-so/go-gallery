package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

type collectionGetByUserIDInput struct {
	UserID persist.DBID `form:"user_id" json:"user_id" binding:"required"`
}
type collectionGetByIDInput struct {
	ID persist.DBID `form:"id" json:"id" binding:"required"`
}
type collectionGetByIDOutput struct {
	Collection *persist.Collection `json:"collection"`
}

type collectionGetOutput struct {
	Collections []*persist.Collection `json:"collections"`
}

type collectionCreateInput struct {
	GalleryID      persist.DBID   `json:"gallery_id" binding:"required"`
	Nfts           []persist.DBID `json:"nfts" binding:"required"`
	Name           string         `json:"name"`
	CollectorsNote string         `json:"collectors_note"`
}

type collectionUpdateInfoByIDInput struct {
	ID             persist.DBID `json:"id" binding:"required"`
	Name           string       `json:"name" binding:"required"`
	CollectorsNote string       `json:"collectors_note"`
}

type collectionUpdateHiddenByIDInput struct {
	ID     persist.DBID `json:"id" binding:"required"`
	Hidden bool         `json:"hidden"`
}
type collectionUpdateNftsByIDinput struct {
	ID   persist.DBID   `json:"id" binding:"required"`
	Nfts []persist.DBID `json:"nfts" binding:"required"`
}

type collectionCreateOutput struct {
	ID persist.DBID `json:"collection_id"`
}

type collectionDeleteInput struct {
	ID persist.DBID `json:"id" binding:"required"`
}

// HANDLERS

func getCollectionsByUserID(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &collectionGetByUserIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{
				Error: err.Error(),
			})
			return
		}

		auth := c.GetBool(authContextKey)
		colls, err := persist.CollGetByUserID(c, input.UserID, auth, pRuntime)
		if len(colls) == 0 || err != nil {
			colls = []*persist.Collection{}
		}

		c.JSON(http.StatusOK, collectionGetOutput{Collections: colls})

	}
}

func getCollectionByID(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &collectionGetByIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{
				Error: err.Error(),
			})
			return
		}

		auth := c.GetBool(authContextKey)
		colls, err := persist.CollGetByID(c, input.ID, auth, pRuntime)
		if len(colls) == 0 || err != nil {
			c.JSON(http.StatusNotFound, errorResponse{
				Error: fmt.Sprintf("no collections found with id: %s", input.ID),
			})
			return
		}
		if len(colls) > 1 {
			colls = colls[:1]
			// TODO log that this should not be happening
		}

		c.JSON(http.StatusOK, collectionGetByIDOutput{Collection: colls[0]})
		return

	}
}

//------------------------------------------------------------

func createCollection(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &collectionCreateInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{
				Error: err.Error(),
			})
			return
		}

		userID, ok := getUserIDfromCtx(c)
		if !ok {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "user id not found in context"})
			return
		}

		//------------------
		// CREATE

		id, err := collectionCreateDb(c, input, userID, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{
				Error: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, collectionCreateOutput{ID: id})
	}
}

func updateCollectionInfo(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateInfoByIDInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		userID, ok := getUserIDfromCtx(c)
		if !ok {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "user id not found in context"})
			return
		}

		update := &persist.CollectionUpdateInfoInput{Name: input.Name, CollectorsNote: input.CollectorsNote}

		err := persist.CollUpdate(c, input.ID, userID, update, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, successOutput{Success: true})
	}
}

func updateCollectionHidden(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateHiddenByIDInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		userID, ok := getUserIDfromCtx(c)
		if !ok {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "user id not found in context"})
			return
		}

		update := &persist.CollectionUpdateHiddenInput{Hidden: input.Hidden}

		err := persist.CollUpdate(c, input.ID, userID, update, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, successOutput{Success: true})
	}
}

func updateCollectionNfts(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateNftsByIDinput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		userID, ok := getUserIDfromCtx(c)
		if !ok {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "user id not found in context"})
			return
		}

		update := &persist.CollectionUpdateNftsInput{Nfts: input.Nfts}

		err := persist.CollUpdateNFTs(c, input.ID, userID, update, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, successOutput{Success: true})
	}
}

func deleteCollection(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionDeleteInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{
				Error: err.Error(),
			})
			return
		}

		userID, ok := getUserIDfromCtx(c)
		if !ok {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "user id not found in context"})
			return
		}

		err := persist.CollDelete(c, input.ID, userID, pRuntime)
		if err != nil {
			switch err.(type) {
			case *persist.DocumentNotFoundError:
				c.JSON(http.StatusNotFound, errorResponse{
					Error: err.Error(),
				})
				return

			default:
				c.JSON(http.StatusInternalServerError, errorResponse{
					Error: err.Error(),
				})
				return
			}
		}

		c.JSON(http.StatusOK, successOutput{Success: true})
	}
}

// CREATE
func collectionCreateDb(pCtx context.Context, pInput *collectionCreateInput,
	pUserID persist.DBID,
	pRuntime *runtime.Runtime) (persist.DBID, error) {

	coll := &persist.CollectionDB{
		OwnerUserID:    pUserID,
		Nfts:           pInput.Nfts,
		Name:           pInput.Name,
		CollectorsNote: pInput.CollectorsNote,
	}

	collID, err := persist.CollCreate(pCtx, coll, pRuntime)
	if err != nil {
		return "", err
	}

	err = persist.GalleryAddCollections(pCtx, pInput.GalleryID, pUserID, []persist.DBID{collID}, pRuntime)
	if err != nil {
		return "", err
	}

	return collID, nil

}
