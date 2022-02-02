package admin

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

var errMustProvideUserIdentifier = fmt.Errorf("must provide either ID or username")
var errNoGalleries = errors.New("no galleries found for first user")

type getUserInput struct {
	ID       persist.DBID `form:"id"`
	Username string       `form:"username"`
}
type deleteUserInput struct {
	ID persist.DBID `json:"id" binding:"required"`
}

type updateUserInput struct {
	ID        persist.DBID `json:"id" binding:"required"`
	Username  string       `json:"username" binding:"required"`
	Bio       string       `json:"bio" binding:"required"`
	Addresses []string     `json:"addresses" binding:"required"`
}

type mergeUserInput struct {
	FirstUserID  persist.DBID `json:"first_user" binding:"required"`
	SecondUserID persist.DBID `json:"second_user" binding:"required"`
}

type createUserInput struct {
	Addresses []persist.Address `json:"addresses" binding:"required"`
	Username  string            `json:"username" binding:"required"`
	Bio       string            `json:"bio"`
}

type createUserOutput struct {
	UserID    persist.DBID `json:"user_id"`
	GalleryID persist.DBID `json:"gallery_id"`
}

func getUser(getUserByIDStmt, getUserByUsername *sql.Stmt) gin.HandlerFunc {

	return func(c *gin.Context) {
		var input getUserInput
		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		var user persist.User
		var err error
		if input.ID != "" {
			err = getUserByIDStmt.QueryRowContext(c, input.ID).Scan(&user.ID, pq.Array(&user.Addresses), &user.Bio, &user.Username, &user.UsernameIdempotent, &user.LastUpdated, &user.CreationTime)
		} else if input.Username != "" {
			err = getUserByUsername.QueryRowContext(c, input.Username).Scan(&user.ID, pq.Array(&user.Addresses), &user.Bio, &user.Username, &user.UsernameIdempotent, &user.LastUpdated, &user.CreationTime)
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

func createUser(db *sql.DB, createUserStmt, createGalleryStmt, createNonceStmt *sql.Stmt) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input createUserInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		tx, err := db.BeginTx(c, nil)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		var userID, galleryID persist.DBID
		if err := tx.StmtContext(c, createUserStmt).QueryRowContext(c, persist.GenerateID(), pq.Array(input.Addresses), input.Username, strings.ToLower(input.Username), input.Bio).Scan(&userID); err != nil {
			rollbackWithErr(c, tx, http.StatusInternalServerError, err)
			return
		}

		if err := tx.StmtContext(c, createGalleryStmt).QueryRowContext(c, persist.GenerateID(), userID, pq.Array([]persist.DBID{})).Scan(&galleryID); err != nil {
			rollbackWithErr(c, tx, http.StatusInternalServerError, err)
			return
		}

		for _, address := range input.Addresses {
			if _, err := tx.StmtContext(c, createNonceStmt).ExecContext(c, persist.GenerateID(), userID, address, auth.GenerateNonce()); err != nil {
				rollbackWithErr(c, tx, http.StatusInternalServerError, err)
				return
			}
		}

		if err := tx.Commit(); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, createUserOutput{
			UserID:    userID,
			GalleryID: galleryID,
		})
	}
}

func updateUser(updateUserStmt *sql.Stmt) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input updateUserInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		if _, err := updateUserStmt.ExecContext(c, pq.Array(input.Addresses), input.Bio, input.Username, strings.ToLower(input.Username), input.ID); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
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

		if _, err := tx.StmtContext(c, deleteUserStmt).ExecContext(c, input.ID); err != nil {
			rollbackWithErr(c, tx, http.StatusInternalServerError, err)
			return
		}

		res, err := getGalleriesStmt.QueryContext(c, input.ID)
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

func mergeUser(db *sql.DB, getUserByIDStmt, updateUserStmt, deleteUserStmt, getGalleriesStmt, deleteGalleriesStmt, updateGalleryStmt *sql.Stmt) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input mergeUserInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		tx, err := db.BeginTx(c, nil)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		var firstUser persist.User
		if err := getUserByIDStmt.QueryRowContext(c, input.FirstUserID).Scan(&firstUser.ID, pq.Array(&firstUser.Addresses), &firstUser.Bio, &firstUser.Username, &firstUser.UsernameIdempotent, &firstUser.LastUpdated, &firstUser.CreationTime); err != nil {
			rollbackWithErr(c, tx, http.StatusInternalServerError, err)
			return
		}

		var secondUser persist.User
		if err := getUserByIDStmt.QueryRowContext(c, input.SecondUserID).Scan(&secondUser.ID, pq.Array(&secondUser.Addresses), &secondUser.Bio, &secondUser.Username, &secondUser.UsernameIdempotent, &secondUser.LastUpdated, &secondUser.CreationTime); err != nil {
			rollbackWithErr(c, tx, http.StatusInternalServerError, err)
			return
		}

		if _, err := tx.StmtContext(c, updateUserStmt).ExecContext(c, pq.Array(append(firstUser.Addresses, secondUser.Addresses...)), firstUser.Bio, firstUser.Username, firstUser.UsernameIdempotent, persist.LastUpdatedTime{}, firstUser.ID); err != nil {
			rollbackWithErr(c, tx, http.StatusInternalServerError, err)
			return
		}

		res, err := getGalleriesStmt.QueryContext(c, input.FirstUserID)
		if err != nil {
			rollbackWithErr(c, tx, http.StatusInternalServerError, err)
			return
		}
		defer res.Close()

		galleries := make([]persist.GalleryDB, 0, 1)
		for res.Next() {
			var g persist.GalleryDB
			if err := res.Scan(&g.ID, pq.Array(&g.Collections)); err != nil {
				rollbackWithErr(c, tx, http.StatusInternalServerError, err)
				return
			}
			galleries = append(galleries, g)
		}

		if err := res.Err(); err != nil {
			rollbackWithErr(c, tx, http.StatusInternalServerError, err)
			return
		}

		nextRes, err := getGalleriesStmt.QueryContext(c, input.SecondUserID)
		if err != nil {
			rollbackWithErr(c, tx, http.StatusInternalServerError, err)
			return
		}
		defer nextRes.Close()

		secondGalleries := make([]persist.GalleryDB, 0, 1)
		for nextRes.Next() {
			var g persist.GalleryDB
			if err := nextRes.Scan(&g.ID, pq.Array(&g.Collections)); err != nil {
				rollbackWithErr(c, tx, http.StatusInternalServerError, err)
				return
			}
			secondGalleries = append(secondGalleries, g)
		}

		if err := nextRes.Err(); err != nil {
			rollbackWithErr(c, tx, http.StatusInternalServerError, err)
			return
		}

		if len(galleries) == 0 {
			rollbackWithErr(c, tx, http.StatusInternalServerError, errNoGalleries)
			return
		}
		gallery := galleries[0]
		if len(secondGalleries) > 0 {
			delStmt := tx.StmtContext(c, deleteGalleriesStmt)
			for _, g := range secondGalleries {
				gallery.Collections = append(gallery.Collections, g.Collections...)
				if _, err := delStmt.ExecContext(c, g.ID); err != nil {
					rollbackWithErr(c, tx, http.StatusInternalServerError, err)
					return
				}
			}
		}

		if _, err := tx.StmtContext(c, updateGalleryStmt).ExecContext(c, pq.Array(gallery.Collections), gallery.ID); err != nil {
			rollbackWithErr(c, tx, http.StatusInternalServerError, err)
			return
		}

		if _, err := tx.StmtContext(c, deleteUserStmt).ExecContext(c, input.SecondUserID); err != nil {
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
