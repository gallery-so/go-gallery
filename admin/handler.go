package admin

import (
	"database/sql"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
)

func handlersInit(router *gin.Engine, db *sql.DB, stmts *statements, ethcl *ethclient.Client, stg *storage.Client) *gin.Engine {
	api := router.Group("/admin/v1")

	users := api.Group("/users")
	users.GET("/get", getUser(stmts.getUserByIDStmt, stmts.getUserByUsernameStmt))
	users.POST("/merge", mergeUser(db, stmts.getUserByIDStmt, stmts.updateUserStmt, stmts.deleteUserStmt, stmts.getGalleriesRawStmt, stmts.deleteGalleryStmt, stmts.updateGalleryStmt))
	users.POST("/update", updateUser(stmts.updateUserStmt))
	users.POST("/delete", deleteUser(db, stmts.deleteUserStmt, stmts.getGalleriesRawStmt, stmts.deleteGalleryStmt, stmts.deleteCollectionStmt))
	users.POST("/create", createUser(db, stmts.createUserStmt, stmts.createGalleryStmt, stmts.createNonceStmt))

	raw := api.Group("/raw")
	raw.POST("/query", queryRaw(db))

	nfts := api.Group("/nfts")
	nfts.GET("/get", getNFTs(stmts.nftRepo))
	nfts.POST("/opensea", refreshOpensea(stmts.nftRepo, stmts.userRepo, stmts.collRepo, stmts.galleryRepo, stmts.backupRepo))
	nfts.GET("/owns", ownsGeneral(ethcl))

	galleries := api.Group("/galleries")
	galleries.GET("/get", getGalleries(stmts.galleryRepo))
	galleries.GET("/refresh", refreshCache(stmts.galleryRepo))
	galleries.GET("/backup", backupGalleries(stmts.galleryRepo, stmts.backupRepo))

	snapshot := api.Group("/snapshot")
	snapshot.GET("/get", getSnapshot(stg))
	snapshot.POST("/update", updateSnapshot(stg))

	collections := api.Group("/collections")
	collections.GET("/get", getCollections(stmts.getCollectionsStmt))
	collections.POST("/update", updateCollection(stmts.updateCollectionStmt))
	collections.POST("/delete", deleteCollection(stmts.deleteCollectionStmt))

	return router
}
