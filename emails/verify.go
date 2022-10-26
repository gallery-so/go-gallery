package emails

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sendgrid/sendgrid-go"
	"github.com/spf13/viper"
)

type VerifyEmailInput struct {
	JWT string `json:"jwt" binding:"required"`
}

type VerifyEmailOutput struct {
	UserID persist.DBID `json:"user_id"`
	Email  string       `json:"email"`
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

		user, err := queries.GetUserById(c, userID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		err = addEmailToSendgridList(c, user.Email.String(), viper.GetString("SENDGRID_DEFAULT_LIST_ID"))
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		err = queries.UpdateUserVerificationStatus(c, coredb.UpdateUserVerificationStatusParams{
			ID:            userID,
			EmailVerified: persist.EmailVerificationStatusVerified,
		})
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, VerifyEmailOutput{
			UserID: user.ID,
			Email:  user.Email.String(),
		})
	}
}

/* example
apiKey := os.Getenv("SENDGRID_API_KEY")
        host := "https://api.sendgrid.com"
        request := sendgrid.GetRequest(apiKey, "/v3/marketing/contacts", host)
        request.Method = "PUT"
        request.Body = []byte(`{
  "contacts": [
    {
      "email": "ryan39@lee-young.com",
      "custom_fields": {
        "w1": "2002-10-02T15:00:00Z",
        "w33": 9.5,
        "e2": "Coffee is a beverage that puts one to sleep when not drank."
      }
    }
  ]
}`)
        response, err := sendgrid.API(request)
        if err != nil {
                log.Println(err)
        } else {
                fmt.Println(response.StatusCode)
                fmt.Println(response.Body)
                fmt.Println(response.Headers)
        }
*/

type sendgridContacts struct {
	ListIDs  []string          `json:"list_ids"`
	Contacts []sendgridContact `json:"contacts"`
}

type sendgridContact struct {
	Email        string                 `json:"email"`
	CustomFields map[string]interface{} `json:"custom_fields"`
}

func addEmailToSendgridList(ctx context.Context, email string, listID string) error {

	request := sendgrid.GetRequest(viper.GetString("SENDGRID_API_KEY"), "/v3/marketing/contacts", "https://api.sendgrid.com")
	request.Method = "PUT"

	contacts := sendgridContacts{
		ListIDs: []string{listID},
		Contacts: []sendgridContact{
			{
				Email: email,
			},
		},
	}

	body, err := json.Marshal(contacts)
	if err != nil {
		return err
	}

	request.Body = body

	response, err := sendgrid.API(request)
	if err != nil {
		return err
	}

	if response.StatusCode != 202 {
		return fmt.Errorf("email contact addition failed and returned: %+v", response)
	}

	return nil

}
