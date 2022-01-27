package admin

import (
	"database/sql"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
)

func handlersInit(router *gin.Engine, db *sql.DB, stmts *statements, ethcl *ethclient.Client) *gin.Engine {
	api := router.Group("/admin/v1")

	users := api.Group("/users")
	users.GET("/get", getUser(stmts.getUserByIDStmt, stmts.getUserByUsernameStmt))
	users.POST("/merge", mergeUser(db, stmts.getUserByIDStmt, stmts.updateUserStmt, stmts.deleteUserStmt, stmts.getGalleriesStmt, stmts.deleteGalleryStmt, stmts.updateGalleryStmt))
	users.POST("/update", updateUser(stmts.updateUserStmt))
	users.POST("/delete", deleteUser(db, stmts.deleteUserStmt, stmts.getGalleriesStmt, stmts.deleteGalleryStmt, stmts.deleteCollectionStmt))

	raw := api.Group("/raw")
	raw.GET("/query", queryRaw(db))

	return router
}
