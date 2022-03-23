package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

type getBackupsInput struct {
	UserID persist.DBID `form:"user_id" binding:"required"`
}

type getBackupsOutput struct {
	Backups []persist.Backup `json:"backups"`
}

type restoreBackupInput struct {
	BackupID persist.DBID `form:"backup_id" binding:"required"`
}

func getBackups(backupRepo persist.BackupRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input getBackupsInput
		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		backups, err := backupRepo.Get(c, input.UserID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, getBackupsOutput{Backups: backups})
	}
}

func restoreBackup(backupRepo persist.BackupRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input restoreBackupInput
		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID := auth.GetUserIDFromCtx(c)

		err := backupRepo.Restore(c, input.BackupID, userID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}
