package admin

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

type getCollectionsInput struct {
	UserID persist.DBID `form:"user_id" binding:"required"`
}

type updateCollectionInput struct {
	ID             persist.DBID   `json:"id" binding:"required"`
	NFTs           []persist.DBID `json:"nfts" binding:"required"`
	Name           string         `json:"name"`
	CollectorsNote string         `json:"collectors_note"`
	Columns        int            `json:"layout"`
	Hidden         bool           `json:"hidden"`
}

type deleteCollectionInput struct {
	ID persist.DBID `json:"id" binding:"required"`
}

func getCollections(getCollsStmt *sql.Stmt) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input getCollectionsInput
		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		colls := make([]persist.Collection, 0, 5)
		res, err := getCollsStmt.QueryContext(c, input.UserID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		defer res.Close()

		for res.Next() {
			var coll persist.Collection
			if err := res.Scan(&coll.ID, &coll.OwnerUserID, pq.Array(&coll.NFTs), &coll.Name, &coll.CollectorsNote, &coll.Layout, &coll.Hidden, &coll.Version, &coll.CreationTime, &coll.LastUpdated); err != nil {
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}
			colls = append(colls, coll)
		}
		if err := res.Err(); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, colls)
	}

}

func updateCollection(updateCollStmt *sql.Stmt) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input updateCollectionInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		if _, err := updateCollStmt.ExecContext(c, input.ID, input.Name, input.CollectorsNote, input.Columns, input.Hidden, persist.LastUpdatedTime{}); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})

	}
}

func deleteCollection(deleteCollStmt *sql.Stmt) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input deleteCollectionInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		if _, err := deleteCollStmt.ExecContext(c, input.ID); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}
