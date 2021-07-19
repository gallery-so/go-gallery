package server

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

type collectionGetByIdInput struct {
	Id persist.DbId `json:"id" binding:"required"       `
}

type collectionGetByUserIdInput struct {
	UserId persist.DbId `json:"user_id" binding:"required"       `
}

type collectionCreateInput struct {
	Nfts []persist.DbId `json:"nfts" binding:"required"`
}

type collectionUpdateByIdInput struct {
	Id     persist.DbId   `json:"id" binding:"required"`
	Name   string         `json:"name,omitempty"`
	Nfts   []*persist.Nft `json:"nfts,omitempty"`
	Hidden bool           `json:"hidden,omitempty"`
}
type collectionCreateOutput struct {
	Id persist.DbId `json:"collection_id"`
}

type collectionDeleteInput struct {
	IDstr persist.DbId `json:"id"`
}

//-------------------------------------------------------------
// HANDLERS

func getAllCollectionsForUser(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		//------------------
		// INPUT

		input := &collectionGetByUserIdInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		colls, err := persist.CollGetByUserID(input.UserId, c, pRuntime)
		if len(colls) == 0 || err != nil {
			colls = []*persist.Collection{}
		}

		c.JSON(http.StatusOK, gin.H{"collections": colls})
	}
}

func createCollection(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &collectionCreateInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ownerId := c.GetString(userIdContextKey)

		//------------------
		// CREATE

		id, err := collectionCreateDb(input, persist.DbId(ownerId), c, pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, collectionCreateOutput{Id: id})
	}
}

func updateCollection(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &collectionUpdateByIdInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}

		userId := c.GetString(userIdContextKey)

		coll := &persist.Collection{HiddenBool: input.Hidden}

		if input.Name != "" {
			coll.NameStr = input.Name
		}
		if input.Nfts != nil {
			coll.NFTsLst = input.Nfts
		}

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
		input := collectionCreateInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// TODO make a db func for delete

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
		NFTsLst:        pInput.Nfts,
	}

	return persist.CollCreate(coll, pCtx, pRuntime)

}

//-------------------------------------------------------------
