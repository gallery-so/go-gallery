package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

type feedGetByUserIDInput struct {
	UserID string `form:"user_id" binding:"required"`
}
type feedGetByUserIDOutput struct {
	Feed *persist.Feed `json:"feed"`
}

func getFeedByUserID(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &feedGetByUserIDInput{}
		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{
				Error: err.Error(),
			})
			return
		}

		feeds, err := persist.FeedGetByUserID(c, persist.DBID(input.UserID), pRuntime)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{
				Error: err.Error(),
			})
			return
		}
		if len(feeds) == 0 {
			c.JSON(http.StatusNotFound, errorResponse{
				Error: "no feed found",
			})
			return
		}
		if len(feeds) > 0 {
			c.JSON(http.StatusInternalServerError, errorResponse{
				Error: "too many feeds returned",
			})
			return
		}

		c.JSON(http.StatusOK, feedGetByUserIDOutput{Feed: feeds[0]})

	}
}
