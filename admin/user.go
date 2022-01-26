package admin

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

var errMustProvideUserIdentifier = fmt.Errorf("must provide either ID or username")

type getUserInput struct {
	ID       persist.DBID `json:"id"`
	Username string       `json:"username"`
}
type deleteUserInput struct {
	ID persist.DBID `json:"id" binding:"required"`
}

func getUser(getUserByIDStmt, getUserByUsername *sql.Stmt) gin.HandlerFunc {

	return func(c *gin.Context) {
		var input getUserInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		var user persist.User
		var err error
		if input.ID != "" {
			err = getUserByIDStmt.QueryRowContext(c, input.ID).Scan(&user.ID, pq.Array(&user.Addresses), &user.Bio, &user.Username, &user.UsernameIdempotent, &user.LastUpdated, &user.CreationTime, &user.Deleted)
		} else if input.Username != "" {
			err = getUserByUsername.QueryRowContext(c, input.Username).Scan(&user.ID, pq.Array(&user.Addresses), &user.Bio, &user.Username, &user.UsernameIdempotent, &user.LastUpdated, &user.CreationTime, &user.Deleted)
		} else {
			util.ErrResponse(c, http.StatusBadRequest, errMustProvideUserIdentifier)
			return
		}

		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, user)
	}
}

func deleteUser(db *sql.DB, deleteUserStmt, getGalleriesStmt, deleteGalleryStmt, deleteCollectionStmt *sql.Stmt) gin.HandlerFunc {
	return func(c *gin.Context) {

		var input deleteUserInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		tx, err := db.BeginTx(c, nil)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if _, err := tx.StmtContext(c, deleteGalleryStmt).ExecContext(c, input.ID); err != nil {
			rollbackWithErr(c, tx, http.StatusInternalServerError, err)
			return
		}

		res, err := tx.StmtContext(c, getGalleriesStmt).QueryContext(c, input.ID)
		if err != nil {
			rollbackWithErr(c, tx, http.StatusInternalServerError, err)
			return
		}
		defer res.Close()

		for res.Next() {
			var g persist.GalleryDB
			if err := res.Scan(&g.ID, pq.Array(&g.Collections)); err != nil {
				rollbackWithErr(c, tx, http.StatusInternalServerError, err)
				return
			}
			for _, coll := range g.Collections {
				if _, err := tx.StmtContext(c, deleteCollectionStmt).ExecContext(c, coll); err != nil {
					rollbackWithErr(c, tx, http.StatusInternalServerError, err)
					return
				}
			}
			if _, err := tx.StmtContext(c, deleteGalleryStmt).ExecContext(c, g.ID); err != nil {
				rollbackWithErr(c, tx, http.StatusInternalServerError, err)
				return
			}
		}

		if err := res.Err(); err != nil {
			rollbackWithErr(c, tx, http.StatusInternalServerError, err)
			return
		}

		if err := tx.Commit(); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}
