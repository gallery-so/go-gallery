package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

type collectionGetByUserIdInput struct {
	UserId persist.DbId `form:"user_id" json:"user_id" binding:"required"`
}
type collectionGetByIdInput struct {
	Id persist.DbId `form:"id" json:"id" binding:"required"`
}

type collectionGetOutput struct {
	Collections []*persist.Collection `json:"collections"`
}

type collectionCreateInput struct {
	Nfts []persist.DbId `json:"nfts" binding:"required"`
}

type collectionUpdateInfoByIdInput struct {
	Id             persist.DbId `json:"id" binding:"required"`
	Name           string       `json:"name" binding:"required"`
	CollectorsNote string       `json:"collectors_note"`
}

type collectionUpdateHiddenByIdInput struct {
	Id     persist.DbId `json:"id" binding:"required"`
	Hidden bool         `json:"hidden" binding:"required"`
}
type collectionUpdateNftsByIdInput struct {
	Id   persist.DbId   `json:"id" binding:"required"`
	Nfts []persist.DbId `json:"nfts" binding:"required"`
}

type collectionCreateOutput struct {
	Id persist.DbId `json:"collection_id"`
}

type collectionDeleteInput struct {
	Id persist.DbId `json:"id" binding:"required"`
}

//-------------------------------------------------------------
// HANDLERS

func getCollectionsByUserId(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &collectionGetByUserIdInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		auth := c.GetBool(authContextKey)
		colls, err := persist.CollGetByUserID(input.UserId, auth, c, pRuntime)
		if len(colls) == 0 || err != nil {
			colls = []*persist.Collection{}
		}

		c.JSON(http.StatusOK, collectionGetOutput{Collections: colls})

	}
}

//-------------------------------------------------------------
func getCollectionById(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &collectionGetByIdInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		auth := c.GetBool(authContextKey)
		colls, err := persist.CollGetByID(input.Id, auth, c, pRuntime)
		if len(colls) == 0 || err != nil {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: fmt.Sprintf("no collections found with id: %s", input.Id),
			})
			return
		}
		if len(colls) > 1 {
			colls = colls[:1]
			// TODO log that this should not be happening
		}

		c.JSON(http.StatusOK, collectionGetOutput{Collections: colls})
		return

	}
}

//------------------------------------------------------------

func createCollection(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &collectionCreateInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		userId, ok := getUserIdFromCtx(c)
		if !ok {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "user id not found in context"})
			return
		}

		//------------------
		// CREATE

		id, err := collectionCreateDb(input, userId, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, collectionCreateOutput{Id: id})
	}
}

func updateCollectionInfo(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateInfoByIdInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}

		userId, ok := getUserIdFromCtx(c)
		if !ok {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "user id not found in context"})
			return
		}

		update := &persist.CollectionUpdateInfoInput{NameStr: input.Name, CollectorsNoteStr: input.CollectorsNote}

		err := persist.CollUpdate(input.Id, userId, update, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, successOutput{Success: true})
	}
}

func updateCollectionHidden(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateHiddenByIdInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}

		userId, ok := getUserIdFromCtx(c)
		if !ok {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "user id not found in context"})
			return
		}

		update := &persist.CollectionUpdateHiddenInput{HiddenBool: input.Hidden}

		err := persist.CollUpdate(input.Id, userId, update, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, successOutput{Success: true})
	}
}

func updateCollectionNfts(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateNftsByIdInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}

		userId, ok := getUserIdFromCtx(c)
		if !ok {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "user id not found in context"})
			return
		}

		coll := &persist.CollectionUpdateNftsInput{NftsLst: input.Nfts}

		err := persist.CollUpdate(input.Id, userId, coll, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, successOutput{Success: true})
	}
}

func deleteCollection(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := collectionDeleteInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		userId, ok := getUserIdFromCtx(c)
		if !ok {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "user id not found in context"})
			return
		}

		err := persist.CollDelete(input.Id, userId, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, successOutput{Success: true})
	}
}

//-------------------------------------------------------------
// CREATE
func collectionCreateDb(pInput *collectionCreateInput,
	pUserIDstr persist.DbId,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (persist.DbId, error) {

	coll := &persist.CollectionDb{
		OwnerUserIDstr: pUserIDstr,
		NftsLst:        pInput.Nfts,
	}

	return persist.CollCreate(coll, pCtx, pRuntime)

}

//-------------------------------------------------------------
