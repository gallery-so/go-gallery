package admin

import (
	"database/sql"

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
}

func newStatements(db *sql.DB) *statements {

	getUserByIDStmt, err := db.Prepare(`SELECT ID, ADDRESSES, BIO, USERNAME, USERNAME_IDEMPOTENT, LAST_UPDATED, CREATION_TIME, DELETED FROM USERS WHERE ID = $1`)
	checkNoErr(err)

	getUserByUsernameStmt, err := db.Prepare(`SELECT ID, ADDRESSES, BIO, USERNAME, USERNAME_IDEMPOTENT, LAST_UPDATED, CREATION_TIME, DELETED FROM USERS WHERE USERNAME_IDEMPOTENT = $1`)
	checkNoErr(err)

	deleteUserStmt, err := db.Prepare(`UPDATE users SET DELETED = true WHERE id = $1`)
	checkNoErr(err)

	getGalleriesStmt, err := db.Prepare(`SELECT ID, COLLECTIONS FROM galleries WHERE OWNER_USER_ID = $1`)
	checkNoErr(err)

	deleteGalleryStmt, err := db.Prepare(`UPDATE galleries SET DELETED = true WHERE ID = $1`)
	checkNoErr(err)

	deleteCollectionStmt, err := db.Prepare(`UPDATE collections SET DELETED = true WHERE ID = $1`)
	checkNoErr(err)

	return &statements{
		getUserByIDStmt:       getUserByIDStmt,
		getUserByUsernameStmt: getUserByUsernameStmt,
		deleteUserStmt:        deleteUserStmt,
		getGalleriesStmt:      getGalleriesStmt,
		deleteGalleryStmt:     deleteGalleryStmt,
		deleteCollectionStmt:  deleteCollectionStmt,
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
