package emails

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/emails"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sendgrid/sendgrid-go"
)

func init() {
	env.RegisterValidation("SENDGRID_DEFAULT_LIST_ID", "required")
	env.RegisterValidation("SENDGRID_API_KEY", "required")
}

func preverifyEmail() gin.HandlerFunc {
	return func(c *gin.Context) {
		var input emails.PreverifyEmailInput
		err := c.ShouldBindQuery(&input)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		result, err := validateEmail(c, input.Email, input.Source)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		var preverifyEmailResult emails.PreverifyEmailResult

		switch strings.ToLower(result.Result.Verdict) {
		case "valid":
			preverifyEmailResult = emails.PreverifyEmailResultValid
		case "risky":
			preverifyEmailResult = emails.PreverifyEmailResultRisky
		case "invalid":
			preverifyEmailResult = emails.PreverifyEmailResultInvalid
		default:
			preverifyEmailResult = emails.PreverifyEmailResultInvalid
		}

		c.JSON(http.StatusOK, emails.PreverifyEmailOutput{
			Result: preverifyEmailResult,
		})

	}
}

func verifyEmail(queries *coredb.Queries) gin.HandlerFunc {

	return func(c *gin.Context) {
		var input emails.VerifyEmailInput
		err := c.ShouldBindJSON(&input)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID, emailFromToken, err := auth.ParseEmailVerificationToken(c, input.JWT)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userWithPII, err := queries.GetUserWithPIIByID(c, userID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if userWithPII.PiiVerifiedEmailAddress.String() != "" {
			util.ErrResponse(c, http.StatusBadRequest, fmt.Errorf("email already verified"))
			return
		}

		if !strings.EqualFold(userWithPII.PiiUnverifiedEmailAddress.String(), emailFromToken) {
			util.ErrResponse(c, http.StatusBadRequest, fmt.Errorf("email does not match"))
			return
		}

		// At this point, the unverified email address has been verified
		verifiedEmail := userWithPII.PiiUnverifiedEmailAddress

		err = queries.UpdateUserVerifiedEmail(c, coredb.UpdateUserVerifiedEmailParams{
			UserID:       userID,
			EmailAddress: verifiedEmail,
		})

		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		err = addEmailToSendgridList(c, verifiedEmail.String(), env.GetString("SENDGRID_DEFAULT_LIST_ID"))
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, emails.VerifyEmailOutput{
			UserID: userWithPII.ID,
			Email:  verifiedEmail,
		})
	}
}

func processAddToMailingList(queries *coredb.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input task.AddEmailToMailingListMessage

		if err := c.ShouldBindJSON(&input); err != nil {
			// Return OK to remove message from queue
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		userWithPII, err := queries.GetUserWithPIIByID(c, input.UserID)
		if err != nil {
			util.ErrResponse(c, http.StatusNotFound, err)
			return
		}

		if userWithPII.PiiVerifiedEmailAddress.String() == "" {
			util.ErrResponse(c, http.StatusOK, fmt.Errorf("email not verified"))
			return
		}

		err = addEmailToSendgridList(c, userWithPII.PiiVerifiedEmailAddress.String(), env.GetString("SENDGRID_DEFAULT_LIST_ID"))
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
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

	request := sendgrid.GetRequest(env.GetString("SENDGRID_API_KEY"), "/v3/marketing/contacts", "https://api.sendgrid.com")
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

type sendgridEmailValidation struct {
	Email  persist.Email `json:"email"`
	Source string        `json:"source"`
}

/*
{
   "result":{
      "email":"bc@gallery.so",
      "verdict":"Risky",
      "score":0.21029,
      "local":"bc",
      "host":"gallery.so",
      "checks":{
         "domain":{
            "has_valid_address_syntax":true,
            "has_mx_or_a_record":true,
            "is_suspected_disposable_address":false
         },
         "local_part":{
            "is_suspected_role_address":false
         },
         "additional":{
            "has_known_bounces":false,
            "has_suspected_bounces":true
         }
      },
      "source":"SIGNUP",
      "ip_address":"172.119.250.71"
   }
}
*/

type sendgridEmailValidationResult struct {
	Result struct {
		Email   string  `json:"email"`
		Verdict string  `json:"verdict"`
		Score   float64 `json:"score"`
		Local   string  `json:"local"`
		Host    string  `json:"host"`
		Checks  struct {
			Domain struct {
				HasValidAddressSyntax        bool `json:"has_valid_address_syntax"`
				HasMxOrARecord               bool `json:"has_mx_or_a_record"`
				IsSuspectedDisposableAddress bool `json:"is_suspected_disposable_address"`
			} `json:"domain"`
			LocalPart struct {
				IsSuspectedRoleAddress bool `json:"is_suspected_role_address"`
			} `json:"local_part"`
			Additional struct {
				HasKnownBounces     bool `json:"has_known_bounces"`
				HasSuspectedBounces bool `json:"has_suspected_bounces"`
			} `json:"additional"`
		} `json:"checks"`
		Source    string `json:"source"`
		IPAddress string `json:"ip_address"`
	} `json:"result"`
}

func validateEmail(ctx context.Context, email persist.Email, source string) (sendgridEmailValidationResult, error) {

	var result sendgridEmailValidationResult

	request := sendgrid.GetRequest(env.GetString("SENDGRID_VALIDATION_KEY"), "/v3/validations/email", "https://api.sendgrid.com")
	request.Method = "POST"

	val := sendgridEmailValidation{
		Email:  email,
		Source: source,
	}

	body, err := json.Marshal(val)
	if err != nil {
		return result, err
	}

	request.Body = body

	response, err := sendgrid.API(request)
	if err != nil {
		return result, err
	}

	if response.StatusCode != 200 {
		return result, fmt.Errorf("verify email returned: %+v", response)
	}

	err = json.Unmarshal([]byte(response.Body), &result)
	if err != nil {
		return result, err
	}

	return result, nil

}
