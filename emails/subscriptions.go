package emails

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sendgrid/sendgrid-go"
	"golang.org/x/sync/errgroup"
)

func init() {
	env.RegisterValidation("SENDGRID_UNSUBSCRIBE_NOTIFICATIONS_GROUP_ID", "required")
	env.RegisterValidation("SENDGRID_API_KEY", "required")
}

var emailTypes = []model.EmailUnsubscriptionType{model.EmailUnsubscriptionTypeAll, model.EmailUnsubscriptionTypeNotifications}

type UpdateSubscriptionsTypeInput struct {
	UserID persist.DBID                 `json:"user_id,required"`
	Unsubs persist.EmailUnsubscriptions `json:"unsubscriptions" binding:"required"`
}

type UnsubInput struct {
	JWT    string                          `json:"jwt,required"`
	Unsubs []model.EmailUnsubscriptionType `json:"unsubscriptions" binding:"required"`
}

type ResubInput struct {
	JWT    string                          `json:"jwt,required"`
	Resubs []model.EmailUnsubscriptionType `json:"resubscriptions" binding:"required"`
}

func updateSubscriptions(queries *coredb.Queries) gin.HandlerFunc {

	return func(c *gin.Context) {
		var input UpdateSubscriptionsTypeInput
		err := c.ShouldBindJSON(&input)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userWithPII, err := queries.GetUserWithPIIByID(c, input.UserID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if userWithPII.PiiEmailAddress.String() == "" {
			util.ErrResponse(c, http.StatusBadRequest, errNoEmailSet{input.UserID})
			return
		}

		emailAddress := userWithPII.PiiEmailAddress.String()

		errGroup := new(errgroup.Group)

		for _, emailType := range emailTypes {
			switch emailType {
			case model.EmailUnsubscriptionTypeAll:
				errGroup.Go(func() error {
					if input.Unsubs.All {
						return addEmailToGlobalUnsubscribeGroup(c, emailAddress)
					}
					return removeEmailFromGlobalUnsubscribeGroup(c, emailAddress)
				})

			case model.EmailUnsubscriptionTypeNotifications:

				errGroup.Go(func() error {
					if input.Unsubs.Notifications {
						return addEmailToUnsubscribeGroup(c, emailAddress, env.GetString("SENDGRID_UNSUBSCRIBE_NOTIFICATIONS_GROUP_ID"))
					}
					return removeEmailFromUnsubscribeGroup(c, emailAddress, env.GetString("SENDGRID_UNSUBSCRIBE_NOTIFICATIONS_GROUP_ID"))
				})
			default:
				util.ErrResponse(c, http.StatusBadRequest, fmt.Errorf("unsupported email type: %s", emailType))
				return
			}

		}

		logger.For(c).Infof("unsubscribing user %s from email types: %+v", input.UserID, input.Unsubs)
		err = queries.UpdateUserEmailUnsubscriptions(c, coredb.UpdateUserEmailUnsubscriptionsParams{
			ID:                   input.UserID,
			EmailUnsubscriptions: input.Unsubs,
		})
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if err := errGroup.Wait(); err != nil {
			logger.For(c).Errorf("error unsubscribing user %s from email types %+v: %s", input.UserID, input.Unsubs, err)
		}

		c.Status(http.StatusOK)
	}
}

func unsubscribe(queries *coredb.Queries) gin.HandlerFunc {

	return func(c *gin.Context) {
		var input UnsubInput
		err := c.ShouldBindJSON(&input)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID, emailFromToken, err := jwtParse(input.JWT)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userWithPII, err := queries.GetUserWithPIIByID(c, userID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if userWithPII.PiiEmailAddress.String() == "" {
			util.ErrResponse(c, http.StatusBadRequest, errNoEmailSet{userID})
			return
		}

		emailAddress := userWithPII.PiiEmailAddress.String()
		if !strings.EqualFold(emailAddress, emailFromToken) {
			util.ErrResponse(c, http.StatusBadRequest, errEmailMismatch{userID})
			return
		}

		unsubs := userWithPII.EmailUnsubscriptions

		errGroup := new(errgroup.Group)

		for _, emailType := range input.Unsubs {
			switch emailType {
			case model.EmailUnsubscriptionTypeAll:
				unsubs.All = true
				errGroup.Go(func() error {
					return addEmailToGlobalUnsubscribeGroup(c, emailAddress)
				})

			case model.EmailUnsubscriptionTypeNotifications:
				unsubs.Notifications = true
				errGroup.Go(func() error {
					return addEmailToUnsubscribeGroup(c, emailAddress, env.GetString("SENDGRID_UNSUBSCRIBE_NOTIFICATIONS_GROUP_ID"))
				})
			default:
				util.ErrResponse(c, http.StatusBadRequest, fmt.Errorf("unsupported email type: %s", emailType))
				return
			}

		}

		logger.For(c).Infof("unsubscribing user %s from email types: %+v", userID, unsubs)
		err = queries.UpdateUserEmailUnsubscriptions(c, coredb.UpdateUserEmailUnsubscriptionsParams{
			ID:                   userID,
			EmailUnsubscriptions: unsubs,
		})
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if err := errGroup.Wait(); err != nil {
			logger.For(c).Errorf("error unsubscribing user %s from email types %+v: %s", userID, input.Unsubs, err)
		}

		c.Status(http.StatusOK)
	}
}

func resubscribe(queries *coredb.Queries) gin.HandlerFunc {

	return func(c *gin.Context) {
		var input ResubInput
		err := c.ShouldBindJSON(&input)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userID, emailFromToken, err := jwtParse(input.JWT)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userWithPII, err := queries.GetUserWithPIIByID(c, userID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if userWithPII.PiiEmailAddress.String() == "" {
			util.ErrResponse(c, http.StatusBadRequest, errNoEmailSet{userID})
			return
		}

		emailAddress := userWithPII.PiiEmailAddress.String()
		if !strings.EqualFold(emailAddress, emailFromToken) {
			util.ErrResponse(c, http.StatusBadRequest, errEmailMismatch{userID})
			return
		}

		unsubs := userWithPII.EmailUnsubscriptions

		errGroup := new(errgroup.Group)

		for _, emailType := range input.Resubs {
			switch emailType {
			case model.EmailUnsubscriptionTypeAll:
				unsubs.All = false
				errGroup.Go(func() error {
					return removeEmailFromGlobalUnsubscribeGroup(c, emailAddress)
				})

			case model.EmailUnsubscriptionTypeNotifications:
				unsubs.Notifications = false
				errGroup.Go(func() error {
					return removeEmailFromUnsubscribeGroup(c, emailAddress, env.GetString("SENDGRID_UNSUBSCRIBE_NOTIFICATIONS_GROUP_ID"))
				})
			default:
				util.ErrResponse(c, http.StatusBadRequest, fmt.Errorf("unsupported email type: %s", emailType))
				return
			}

		}

		logger.For(c).Infof("unsubscribing user %s from email types: %+v", userID, unsubs)
		err = queries.UpdateUserEmailUnsubscriptions(c, coredb.UpdateUserEmailUnsubscriptionsParams{
			ID:                   userID,
			EmailUnsubscriptions: unsubs,
		})
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if err := errGroup.Wait(); err != nil {
			logger.For(c).Errorf("error unsubscribing user %s from email types %+v: %s", userID, input.Resubs, err)
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
	request := sendgrid.GetRequest(env.GetString("SENDGRID_API_KEY"), url, "https://api.sendgrid.com")
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
	request := sendgrid.GetRequest(env.GetString("SENDGRID_API_KEY"), url, "https://api.sendgrid.com")
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
