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
	Raw []map[string]interface{} `json:"raw"`
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
			rollbackWithErr(c, tx, http.StatusInternalServerError, err)
			return
		}
		defer res.Close()

		colls, err := res.Columns()
		if err != nil {
			rollbackWithErr(c, tx, http.StatusInternalServerError, err)
			return
		}

		output := queryRawOutput{Raw: make([]map[string]interface{}, 0, 10)}
		for res.Next() {
			raw := make([]interface{}, len(colls))
			for i := 0; i < len(colls); i++ {
				raw[i] = new(interface{})
			}
			if err := res.Scan(raw...); err != nil {
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}
			row := make(map[string]interface{}, len(colls))
			for i := 0; i < len(colls); i++ {
				row[colls[i]] = raw[i]
			}
			output.Raw = append(output.Raw, row)
		}

		if err := tx.Commit(); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, output)
	}

}
