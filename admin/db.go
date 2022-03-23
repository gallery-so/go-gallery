package admin

import (
	"context"
	"database/sql"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
)

type statements struct {
	getUserByIDStmt       *sql.Stmt
	getUserByUsernameStmt *sql.Stmt
	deleteUserStmt        *sql.Stmt
	getGalleriesRawStmt   *sql.Stmt
	deleteGalleryStmt     *sql.Stmt
	deleteCollectionStmt  *sql.Stmt
	updateUserStmt        *sql.Stmt
	updateGalleryStmt     *sql.Stmt
	createUserStmt        *sql.Stmt
	createGalleryStmt     *sql.Stmt
	createNonceStmt       *sql.Stmt
	getCollectionsStmt    *sql.Stmt
	updateCollectionStmt  *sql.Stmt

	galleryRepo persist.GalleryRepository
	nftRepo     persist.NFTRepository
	userRepo    persist.UserRepository
	collRepo    persist.CollectionRepository
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

	getGalleriesRawStmt, err := db.PrepareContext(ctx, `SELECT ID, COLLECTIONS FROM galleries WHERE OWNER_USER_ID = $1;`)
	checkNoErr(err)

	deleteGalleryStmt, err := db.PrepareContext(ctx, `UPDATE galleries SET DELETED = true WHERE ID = $1;`)
	checkNoErr(err)

	deleteCollectionStmt, err := db.PrepareContext(ctx, `UPDATE collections SET DELETED = true WHERE ID = $1;`)
	checkNoErr(err)

	updateUserStmt, err := db.PrepareContext(ctx, `UPDATE users SET ADDRESSES = $1, BIO = $2, USERNAME = $3, USERNAME_IDEMPOTENT = $4, LAST_UPDATED = $5 WHERE ID = $6;`)
	checkNoErr(err)

	updateGalleryStmt, err := db.PrepareContext(ctx, `UPDATE galleries SET COLLECTIONS = $1, LAST_UPDATED = $2 WHERE ID = $3;`)
	checkNoErr(err)

	createUserStmt, err := db.PrepareContext(ctx, `INSERT INTO users (ID, ADDRESSES, USERNAME, USERNAME_IDEMPOTENT, BIO) VALUES ($1, $2, $3, $4, $5) RETURNING ID;`)
	checkNoErr(err)

	createGalleryStmt, err := db.PrepareContext(ctx, `INSERT INTO galleries (ID,OWNER_USER_ID, COLLECTIONS) VALUES ($1, $2, $3) RETURNING ID;`)
	checkNoErr(err)

	createNonceStmt, err := db.PrepareContext(ctx, `INSERT INTO nonces (ID,USER_ID, ADDRESS, VALUE) VALUES ($1, $2, $3, $4);`)
	checkNoErr(err)

	getCollectionsStmt, err := db.PrepareContext(ctx, `SELECT ID,OWNER_USER_ID,NFTS,NAME,COLLECTORS_NOTE,LAYOUT,HIDDEN,VERSION,CREATED_AT,LAST_UPDATED FROM collections WHERE OWNER_USER_ID = $1 AND DELETED = false;`)
	checkNoErr(err)

	updateCollectionStmt, err := db.PrepareContext(ctx, `UPDATE collections SET NFTS = $1, NAME = $2, COLLECTORS_NOTE = $3, LAYOUT = $4, HIDDEN = $5, LAST_UPDATED = $6 WHERE ID = $7;`)
	checkNoErr(err)

	galleryRepo := postgres.NewGalleryRepository(db, nil)
	return &statements{
		getUserByIDStmt:       getUserByIDStmt,
		getUserByUsernameStmt: getUserByUsernameStmt,
		deleteUserStmt:        deleteUserStmt,
		getGalleriesRawStmt:   getGalleriesRawStmt,
		deleteGalleryStmt:     deleteGalleryStmt,
		deleteCollectionStmt:  deleteCollectionStmt,
		updateUserStmt:        updateUserStmt,
		updateGalleryStmt:     updateGalleryStmt,
		createUserStmt:        createUserStmt,
		createGalleryStmt:     createGalleryStmt,
		createNonceStmt:       createNonceStmt,
		getCollectionsStmt:    getCollectionsStmt,
		updateCollectionStmt:  updateCollectionStmt,

		galleryRepo: galleryRepo,
		nftRepo:     postgres.NewNFTRepository(db, galleryRepo),
		userRepo:    postgres.NewUserRepository(db),
		collRepo:    postgres.NewCollectionRepository(db, galleryRepo),
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
