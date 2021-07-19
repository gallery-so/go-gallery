package server

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

type collectionGetByIdInput struct {
	Id persist.DbId `json:"id" binding:"required"`
}

type collectionGetByUserIdInput struct {
	UserId persist.DbId `json:"user_id" binding:"required"`
}
type collectionGetOutput struct {
	Collections []*persist.Collection `json:"nfts"`
}

type collectionCreateInput struct {
	Nfts []persist.DbId `json:"nfts" binding:"required"`
}

type collectionUpdateNameByIdInput struct {
	Id   persist.DbId `json:"id" binding:"required"`
	Name string       `json:"name" binding:"required"`
}

type collectionUpdateHiddenByIdInput struct {
	Id     persist.DbId `json:"id" binding:"required"`
	Hidden bool         `json:"hidden" binding:"required"`
}
type collectionUpdateNftsByIdInput struct {
	Id   persist.DbId   `json:"id" binding:"required"`
	Nfts []*persist.Nft `json:"nfts" binding:"required"`
}

type collectionCreateOutput struct {
	Id persist.DbId `json:"collection_id"`
}

type collectionDeleteInput struct {
	Id persist.DbId `json:"id" binding:"required"`
}

//-------------------------------------------------------------
// HANDLERS

func getAllCollectionsForUser(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &collectionGetByUserIdInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		auth := c.GetBool(authContextKey)

		colls, err := persist.CollGetByUserID(input.UserId, !auth, c, pRuntime)
		if len(colls) == 0 || err != nil {
			colls = []*persist.Collection{}
		}

		c.JSON(http.StatusOK, collectionGetOutput{Collections: colls})
	}
}

func createCollection(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &collectionCreateInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		ownerId := c.GetString(userIdContextKey)

		//------------------
		// CREATE

		id, err := collectionCreateDb(input, persist.DbId(ownerId), c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, collectionCreateOutput{Id: id})
	}
}

func updateCollectionName(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateNameByIdInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}

		userId := c.GetString(userIdContextKey)

		coll := &persist.Collection{NameStr: input.Name}

		err := persist.CollUpdate(input.Id, persist.DbId(userId), coll, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}

		c.Status(http.StatusOK)
	}
}

func updateCollectionHidden(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateHiddenByIdInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}

		userId := c.GetString(userIdContextKey)

		coll := &persist.Collection{HiddenBool: input.Hidden}

		err := persist.CollUpdate(input.Id, persist.DbId(userId), coll, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}

		c.Status(http.StatusOK)
	}
}

func updateCollectionNfts(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateNftsByIdInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}

		userId := c.GetString(userIdContextKey)

		coll := &persist.Collection{NftsLst: input.Nfts}

		err := persist.CollUpdate(input.Id, persist.DbId(userId), coll, c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}

		c.Status(http.StatusOK)
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

		userId := c.GetString(userIdContextKey)

		err := persist.CollDelete(input.Id, persist.DbId(userId), c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: err.Error(),
			})
			return
		}

		c.Status(http.StatusOK)
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
