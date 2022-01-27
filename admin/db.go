package admin

import (
	"context"
	"database/sql"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/util"
)

type statements struct {
	getUserByIDStmt       *sql.Stmt
	getUserByUsernameStmt *sql.Stmt
	deleteUserStmt        *sql.Stmt
	getGalleriesStmt      *sql.Stmt
	deleteGalleryStmt     *sql.Stmt
	deleteCollectionStmt  *sql.Stmt
	updateUserStmt        *sql.Stmt
	updateGalleryStmt     *sql.Stmt
}

func newStatements(db *sql.DB) *statements {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	getUserByIDStmt, err := db.PrepareContext(ctx, `SELECT ID, ADDRESSES, BIO, USERNAME, USERNAME_IDEMPOTENT, LAST_UPDATED, CREATED_AT FROM USERS WHERE ID = $1 AND DELETED = false;`)
	checkNoErr(err)

	getUserByUsernameStmt, err := db.PrepareContext(ctx, `SELECT ID, ADDRESSES, BIO, USERNAME, USERNAME_IDEMPOTENT, LAST_UPDATED, CREATED_AT FROM USERS WHERE USERNAME_IDEMPOTENT = $1 AND DELETED = false;`)
	checkNoErr(err)

	deleteUserStmt, err := db.PrepareContext(ctx, `UPDATE users SET DELETED = true WHERE ID = $1;`)
	checkNoErr(err)

	getGalleriesStmt, err := db.PrepareContext(ctx, `SELECT ID, COLLECTIONS FROM galleries WHERE OWNER_USER_ID = $1;`)
	checkNoErr(err)

	deleteGalleryStmt, err := db.PrepareContext(ctx, `UPDATE galleries SET DELETED = true WHERE ID = $1;`)
	checkNoErr(err)

	deleteCollectionStmt, err := db.PrepareContext(ctx, `UPDATE collections SET DELETED = true WHERE ID = $1;`)
	checkNoErr(err)

	updateUserStmt, err := db.PrepareContext(ctx, `UPDATE users SET ADDRESSES = $1, BIO = $2, USERNAME = $3, USERNAME_IDEMPOTENT = $4, LAST_UPDATED = $5 WHERE ID = $6;`)
	checkNoErr(err)

	updateGalleryStmt, err := db.PrepareContext(ctx, `UPDATE galleries SET COLLECTIONS = $1, LAST_UPDATED = $2 WHERE ID = $3;`)
	checkNoErr(err)

	return &statements{
		getUserByIDStmt:       getUserByIDStmt,
		getUserByUsernameStmt: getUserByUsernameStmt,
		deleteUserStmt:        deleteUserStmt,
		getGalleriesStmt:      getGalleriesStmt,
		deleteGalleryStmt:     deleteGalleryStmt,
		deleteCollectionStmt:  deleteCollectionStmt,
		updateUserStmt:        updateUserStmt,
		updateGalleryStmt:     updateGalleryStmt,
	}

}

func rollbackWithErr(c *gin.Context, tx *sql.Tx, status int, err error) {
	util.ErrResponse(c, status, err)
	tx.Rollback()
}

func checkNoErr(err error) {
	if err != nil {
		panic(err)
	}
}
