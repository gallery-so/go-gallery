package admin

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/util"
)

type queryRawInput struct {
	Query string `json:"query" binding:"required"`
}

type queryRawOutput struct {
	Raw [][]byte `json:"raw"`
}

func queryRaw(db *sql.DB) gin.HandlerFunc {

	return func(c *gin.Context) {

		var input queryRawInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		tx, err := db.BeginTx(c, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		res, err := tx.QueryContext(c, input.Query)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		defer res.Close()

		output := queryRawOutput{Raw: make([][]byte, 0, 10)}
		for res.Next() {
			var raw []byte
			if err := res.Scan(&raw); err != nil {
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}
			output.Raw = append(output.Raw, raw)
		}

		if err := tx.Commit(); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, output)
	}

}
