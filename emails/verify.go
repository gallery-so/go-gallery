package emails

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

type VerifyEmailInput struct {
	JWT    string       `json:"jwt" binding:"required"`
	UserID persist.DBID `json:"user_id" binding:"required"`
}

var errUserIDMismatch = fmt.Errorf("user ID mismatch")

func verifyEmail(queries *coredb.Queries) gin.HandlerFunc {

	return func(c *gin.Context) {
		var input VerifyEmailInput
		err := c.ShouldBindJSON(&input)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID, err := auth.JWTParse(input.JWT, viper.GetString("JWT_SECRET"))
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		if userID != input.UserID {
			util.ErrResponse(c, http.StatusUnauthorized, errUserIDMismatch)
			return
		}

		err = queries.UpdateUserVerificationStatus(c, coredb.UpdateUserVerificationStatusParams{
			ID:            userID,
			EmailVerified: true,
		})
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.Status(http.StatusOK)
	}
}
