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
	users.POST("/delete", deleteUser(db, stmts.deleteUserStmt, stmts.getGalleriesStmt, stmts.deleteGalleryStmt, stmts.deleteCollectionStmt))

	raw := api.Group("/raw")
	raw.GET("/query", queryRaw(db))
	raw.POST("/exec", execRaw(db))

	return router
}
