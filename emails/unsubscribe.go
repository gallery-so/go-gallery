package emails

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sendgrid/sendgrid-go"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
)

type UnsubscribeFromEmailTypeInput struct {
	JWT        string                          `json:"jwt,omitempty"`
	UserID     persist.DBID                    `json:"user_id,omitempty"`
	EmailTypes []model.EmailUnsubscriptionType `json:"email_types" binding:"required"`
}

type ResubscribeFromEmailTypeInput struct {
	JWT       string                          `json:"jwt,omitempty"`
	UserID    persist.DBID                    `json:"user_id,omitempty"`
	EmailType []model.EmailUnsubscriptionType `json:"email_types" binding:"required"`
}

func unsubscribeFromEmailType(queries *coredb.Queries) gin.HandlerFunc {

	return func(c *gin.Context) {
		var input UnsubscribeFromEmailTypeInput
		err := c.ShouldBindJSON(&input)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		if input.JWT == "" && input.UserID == "" {
			util.ErrResponse(c, http.StatusBadRequest, fmt.Errorf("jwt or user_id must be provided"))
			return
		}
		userID := input.UserID
		if input.JWT != "" {
			userID, err = auth.JWTParse(input.JWT, viper.GetString("JWT_SECRET"))
			if err != nil {
				util.ErrResponse(c, http.StatusBadRequest, err)
				return
			}
		}

		user, err := queries.GetUserById(c, userID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if user.Email == "" {
			util.ErrResponse(c, http.StatusBadRequest, errNoEmailSet{userID})
			return
		}

		unsubs := user.EmailUnsubscriptions

		errGroup := new(errgroup.Group)

		for _, emailType := range input.EmailTypes {
			switch emailType {
			case model.EmailUnsubscriptionTypeAll:
				unsubs.All = true
				errGroup.Go(func() error {
					return addEmailToGlobalUnsubscribeGroup(c, user.Email.String())
				})

			case model.EmailUnsubscriptionTypeNotifications:
				unsubs.Notifications = true
				errGroup.Go(func() error {
					return addEmailToUnsubscribeGroup(c, user.Email.String(), viper.GetString("SENDGRID_UNSUBSCRIBE_NOTIFICATIONS_GROUP_ID"))
				})
			default:
				util.ErrResponse(c, http.StatusBadRequest, fmt.Errorf("unsupported email type: %s", emailType))
				return
			}

		}
		err = queries.UpdateUserEmailUnsubscriptions(c, coredb.UpdateUserEmailUnsubscriptionsParams{
			ID:                   userID,
			EmailUnsubscriptions: unsubs,
		})
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if err := errGroup.Wait(); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.Status(http.StatusOK)
	}
}
func resubscribeFromEmailType(queries *coredb.Queries) gin.HandlerFunc {

	return func(c *gin.Context) {
		var input ResubscribeFromEmailTypeInput
		err := c.ShouldBindJSON(&input)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		if input.JWT == "" && input.UserID == "" {
			util.ErrResponse(c, http.StatusBadRequest, fmt.Errorf("jwt or user_id must be provided"))
			return
		}
		userID := input.UserID
		if input.JWT != "" {
			userID, err = auth.JWTParse(input.JWT, viper.GetString("JWT_SECRET"))
			if err != nil {
				util.ErrResponse(c, http.StatusBadRequest, err)
				return
			}
		}

		user, err := queries.GetUserById(c, userID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if user.Email == "" {
			util.ErrResponse(c, http.StatusBadRequest, errNoEmailSet{userID})
			return
		}

		errGroup := new(errgroup.Group)

		unsubs := user.EmailUnsubscriptions

		for _, emailType := range input.EmailType {
			switch emailType {
			case model.EmailUnsubscriptionTypeAll:
				unsubs.All = false
				errGroup.Go(func() error {
					return removeEmailFromGlobalUnsubscribeGroup(c, user.Email.String())
				})
			case model.EmailUnsubscriptionTypeNotifications:
				unsubs.Notifications = false
				errGroup.Go(func() error {
					return removeEmailFromUnsubscribeGroup(c, user.Email.String(), viper.GetString("SENDGRID_UNSUBSCRIBE_NOTIFICATIONS_GROUP_ID"))
				})
			default:
				util.ErrResponse(c, http.StatusBadRequest, fmt.Errorf("unsupported email type: %s", input.EmailType))
				return
			}

		}

		err = queries.UpdateUserEmailUnsubscriptions(c, coredb.UpdateUserEmailUnsubscriptionsParams{
			ID:                   userID,
			EmailUnsubscriptions: unsubs,
		})
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if err := errGroup.Wait(); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.Status(http.StatusOK)
	}
}

type unsubscribeGroupRecipients struct {
	RecipientEmails []string `json:"recipient_emails"`
}

func addEmailToUnsubscribeGroup(ctx context.Context, email string, groupID string) error {
	return unsubscribeSendgrid(ctx, email, fmt.Sprintf("/v3/asm/groups/%s/suppressions", groupID))

}

func addEmailToGlobalUnsubscribeGroup(ctx context.Context, email string) error {
	return unsubscribeSendgrid(ctx, email, "/v3/asm/suppressions/global")
}

func unsubscribeSendgrid(ctx context.Context, email string, url string) error {
	request := sendgrid.GetRequest(viper.GetString("SENDGRID_API_KEY"), url, "https://api.sendgrid.com")
	request.Method = "POST"

	emails := unsubscribeGroupRecipients{
		RecipientEmails: []string{email},
	}

	body, err := json.Marshal(emails)
	if err != nil {
		return err
	}

	request.Body = body

	response, err := sendgrid.API(request)
	if err != nil {
		return err
	}

	if response.StatusCode != 202 {
		return fmt.Errorf("email unsub addition failed and returned: %+v", response)
	}

	return nil

}

func removeEmailFromUnsubscribeGroup(ctx context.Context, email string, groupID string) error {
	return sendSendgridDeleteRequest(ctx, fmt.Sprintf("/v3/asm/groups/%s/suppressions/%s", groupID, email))
}

func removeEmailFromGlobalUnsubscribeGroup(ctx context.Context, email string) error {
	return sendSendgridDeleteRequest(ctx, fmt.Sprintf("/v3/asm/suppressions/global/%s", email))
}

func sendSendgridDeleteRequest(ctx context.Context, url string) error {
	request := sendgrid.GetRequest(viper.GetString("SENDGRID_API_KEY"), url, "https://api.sendgrid.com")
	request.Method = "DELETE"

	response, err := sendgrid.API(request)
	if err != nil {
		return err
	}

	if response.StatusCode != 202 {
		return fmt.Errorf("sendgrid delete request failed and returned: %+v", response)
	}

	return nil

}
