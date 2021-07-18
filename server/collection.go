package server

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

type collectionCreateInput struct {
	NameStr           string         `json:"name"        binding:"required,short_string"`
	CollectorsNoteStr string         `json:"collectors_note" binding:"required,medium_string"`
	Nfts              []persist.DbId `json:"nfts" binding:"required"`
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

		userIDstr := c.Query("user_id")

		colls, err := persist.CollGetByUserID(persist.DbId(userIDstr), c, pRuntime)
		if len(colls) == 0 || err != nil {
			colls = []*persist.Collection{}
		}

		c.JSON(http.StatusOK, gin.H{"collections": colls})
	}
}

func createCollection(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		// TODO sanatize input
		input := &collectionCreateInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ownerId := c.GetString("user_id")

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
		NameStr:           pInput.NameStr,
		CollectorsNoteStr: pInput.CollectorsNoteStr,
		OwnerUserIDstr:    pUserIDstr,
		NFTsLst:           pInput.Nfts,
	}

	return persist.CollCreate(coll, pCtx, pRuntime)

}

//-------------------------------------------------------------
