package emails

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/emails"
	"github.com/mikeydub/go-gallery/service/persist"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sendgrid/sendgrid-go"
	"golang.org/x/sync/errgroup"
)

func init() {
	env.RegisterValidation("SENDGRID_UNSUBSCRIBE_NOTIFICATIONS_GROUP_ID", "required")
	env.RegisterValidation("SENDGRID_UNSUBSCRIBE_DIGEST_GROUP_ID", "required")
	env.RegisterValidation("SENDGRID_UNSUBSCRIBE_MARKETING_GROUP_ID", "required")
	env.RegisterValidation("SENDGRID_UNSUBSCRIBE_MEMBERS_CLUB_GROUP_ID", "required")
	env.RegisterValidation("SENDGRID_API_KEY", "required")
}

func updateUnsubscriptions(queries *coredb.Queries) gin.HandlerFunc {

	return func(c *gin.Context) {
		var input emails.UpdateUnsubscriptionsTypeInput
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

		emailAddress := userWithPII.PiiVerifiedEmailAddress.String()
		if emailAddress == "" {
			emailAddress = userWithPII.PiiUnverifiedEmailAddress.String()
		}

		if emailAddress == "" {
			util.ErrResponse(c, http.StatusBadRequest, errNoEmailSet{input.UserID})
			return
		}

		errGroup := new(errgroup.Group)

		for _, emailType := range model.AllEmailUnsubscriptionType {
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
			case model.EmailUnsubscriptionTypeDigest:
				errGroup.Go(func() error {
					if input.Unsubs.Digest {
						return addEmailToUnsubscribeGroup(c, emailAddress, env.GetString("SENDGRID_UNSUBSCRIBE_DIGEST_GROUP_ID"))
					}
					return removeEmailFromUnsubscribeGroup(c, emailAddress, env.GetString("SENDGRID_UNSUBSCRIBE_DIGEST_GROUP_ID"))
				})
			case model.EmailUnsubscriptionTypeMarketing:
				errGroup.Go(func() error {
					if input.Unsubs.Marketing {
						return addEmailToUnsubscribeGroup(c, emailAddress, env.GetString("SENDGRID_UNSUBSCRIBE_MARKETING_GROUP_ID"))
					}
					return removeEmailFromUnsubscribeGroup(c, emailAddress, env.GetString("SENDGRID_UNSUBSCRIBE_MARKETING_GROUP_ID"))
				})
			case model.EmailUnsubscriptionTypeMembersClub:
				errGroup.Go(func() error {
					if input.Unsubs.MembersClub {
						return addEmailToUnsubscribeGroup(c, emailAddress, env.GetString("SENDGRID_UNSUBSCRIBE_MEMBERS_CLUB_GROUP_ID"))
					}
					return removeEmailFromUnsubscribeGroup(c, emailAddress, env.GetString("SENDGRID_UNSUBSCRIBE_MEMBERS_CLUB_GROUP_ID"))
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

func getUnsubscriptions(queries *coredb.Queries) gin.HandlerFunc {

	digestGroupID := env.GetString("SENDGRID_UNSUBSCRIBE_DIGEST_GROUP_ID")
	notificationsGroupID := env.GetString("SENDGRID_UNSUBSCRIBE_NOTIFICATIONS_GROUP_ID")
	membersClubGroupID := env.GetString("SENDGRID_UNSUBSCRIBE_MEMBERS_CLUB_GROUP_ID")
	marketingGroupID := env.GetString("SENDGRID_UNSUBSCRIBE_MARKETING_GROUP_ID")

	dgidInt, err := strconv.Atoi(digestGroupID)
	if err != nil {
		panic(err)
	}

	ngidInt, err := strconv.Atoi(notificationsGroupID)
	if err != nil {
		panic(err)
	}

	mcgidInt, err := strconv.Atoi(membersClubGroupID)
	if err != nil {
		panic(err)
	}

	mkgidInt, err := strconv.Atoi(marketingGroupID)
	if err != nil {
		panic(err)
	}

	return func(c *gin.Context) {
		var input emails.GetUnsubscriptionsInput
		err := c.ShouldBindQuery(&input)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userWithPII, err := queries.GetUserWithPIIByID(c, input.UserID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		emailAddress := userWithPII.PiiVerifiedEmailAddress.String()
		if emailAddress == "" {
			emailAddress = userWithPII.PiiUnverifiedEmailAddress.String()
		}

		if emailAddress == "" {
			util.ErrResponse(c, http.StatusBadRequest, errNoEmailSet{input.UserID})
			return
		}

		unsubs := userWithPII.EmailUnsubscriptions

		sendgridUnsubs, err := getUnsubscriptionsSendgrid(c, emailAddress)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		globalUnsub, err := getGlobalUnsubscriptionSendgrid(c, emailAddress)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		for _, group := range model.AllEmailUnsubscriptionType {

			switch group {
			case model.EmailUnsubscriptionTypeAll:
				unsubs.All = persist.NullBool(globalUnsub)
			case model.EmailUnsubscriptionTypeNotifications:
				supression, ok := util.FindFirst(sendgridUnsubs.Supressions, func(s sendgridSupressionGroup) bool {
					return s.ID == ngidInt
				})
				unsubs.Notifications = persist.NullBool(ok && supression.Suppressed)

			case model.EmailUnsubscriptionTypeDigest:
				supression, ok := util.FindFirst(sendgridUnsubs.Supressions, func(s sendgridSupressionGroup) bool {
					return s.ID == dgidInt
				})
				unsubs.Digest = persist.NullBool(ok && supression.Suppressed)
			case model.EmailUnsubscriptionTypeMarketing:
				supression, ok := util.FindFirst(sendgridUnsubs.Supressions, func(s sendgridSupressionGroup) bool {
					return s.ID == mkgidInt
				})
				unsubs.Marketing = persist.NullBool(ok && supression.Suppressed)
			case model.EmailUnsubscriptionTypeMembersClub:
				supression, ok := util.FindFirst(sendgridUnsubs.Supressions, func(s sendgridSupressionGroup) bool {
					return s.ID == mcgidInt
				})
				unsubs.MembersClub = persist.NullBool(ok && supression.Suppressed)
			default:
				util.ErrResponse(c, http.StatusBadRequest, fmt.Errorf("unsupported email type: %s", group))
				return
			}

		}

		err = queries.UpdateUserEmailUnsubscriptions(c, coredb.UpdateUserEmailUnsubscriptionsParams{
			ID:                   input.UserID,
			EmailUnsubscriptions: unsubs,
		})
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, emails.GetSubscriptionsResponse{
			Unsubscriptions: unsubs,
		})
	}
}

func unsubscribe(queries *coredb.Queries) gin.HandlerFunc {

	return func(c *gin.Context) {
		var input emails.UnsubInput
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

		if userWithPII.PiiVerifiedEmailAddress.String() == "" {
			util.ErrResponse(c, http.StatusBadRequest, errNoEmailSet{userID})
			return
		}

		emailAddress := userWithPII.PiiVerifiedEmailAddress.String()
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
			case model.EmailUnsubscriptionTypeDigest:
				unsubs.Digest = true
				errGroup.Go(func() error {
					return addEmailToUnsubscribeGroup(c, emailAddress, env.GetString("SENDGRID_UNSUBSCRIBE_DIGEST_GROUP_ID"))
				})
			case model.EmailUnsubscriptionTypeMarketing:
				unsubs.Marketing = true
				errGroup.Go(func() error {
					return addEmailToUnsubscribeGroup(c, emailAddress, env.GetString("SENDGRID_UNSUBSCRIBE_MARKETING_GROUP_ID"))
				})
			case model.EmailUnsubscriptionTypeMembersClub:
				unsubs.MembersClub = true
				errGroup.Go(func() error {
					return addEmailToUnsubscribeGroup(c, emailAddress, env.GetString("SENDGRID_UNSUBSCRIBE_MEMBERS_CLUB_GROUP_ID"))
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
		var input emails.ResubInput
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

		if userWithPII.PiiVerifiedEmailAddress.String() == "" {
			util.ErrResponse(c, http.StatusBadRequest, errNoEmailSet{userID})
			return
		}

		emailAddress := userWithPII.PiiVerifiedEmailAddress.String()
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
			case model.EmailUnsubscriptionTypeDigest:
				unsubs.Digest = false
				errGroup.Go(func() error {
					return removeEmailFromUnsubscribeGroup(c, emailAddress, env.GetString("SENDGRID_UNSUBSCRIBE_DIGEST_GROUP_ID"))
				})
			case model.EmailUnsubscriptionTypeMarketing:
				unsubs.Marketing = false
				errGroup.Go(func() error {
					return removeEmailFromUnsubscribeGroup(c, emailAddress, env.GetString("SENDGRID_UNSUBSCRIBE_MARKETING_GROUP_ID"))
				})
			case model.EmailUnsubscriptionTypeMembersClub:
				unsubs.MembersClub = false
				errGroup.Go(func() error {
					return removeEmailFromUnsubscribeGroup(c, emailAddress, env.GetString("SENDGRID_UNSUBSCRIBE_MEMBERS_CLUB_GROUP_ID"))
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

/*

description
string
The description of the suppression group.

required
id
integer
The id of the suppression group.

required
is_default
boolean
Indicates if the suppression group is set as the default.

required
name
string
The name of the suppression group.

required
suppressed
boolean
Indicates if the given email address is suppressed for this group.

required
*/

type sendgridSupressionGroup struct {
	Description string `json:"description"`
	ID          int    `json:"id"`
	IsDefault   bool   `json:"is_default"`
	Name        string `json:"name"`
	Suppressed  bool   `json:"suppressed"`
}

type getUnsubscriptionsSendgridResponse struct {
	Supressions []sendgridSupressionGroup `json:"suppressions"`
}

func getUnsubscriptionsSendgrid(ctx context.Context, email string) (getUnsubscriptionsSendgridResponse, error) {
	request := sendgrid.GetRequest(env.GetString("SENDGRID_API_KEY"), fmt.Sprintf("/v3/asm/suppressions/%s", email), "https://api.sendgrid.com")
	request.Method = "GET"

	response, err := sendgrid.API(request)
	if err != nil {
		return getUnsubscriptionsSendgridResponse{}, err
	}

	if response.StatusCode >= 300 || response.StatusCode < 200 {
		return getUnsubscriptionsSendgridResponse{}, fmt.Errorf("email unsub addition failed and returned: %+v", response)
	}

	var unsubs getUnsubscriptionsSendgridResponse
	err = json.Unmarshal([]byte(response.Body), &unsubs)
	if err != nil {
		return getUnsubscriptionsSendgridResponse{}, err
	}

	return unsubs, nil
}

type getGlobalUnsubscriptionSendgridResponse struct {
	RecipientEmail string `json:"recipient_email"`
}

func getGlobalUnsubscriptionSendgrid(ctx context.Context, email string) (bool, error) {
	request := sendgrid.GetRequest(env.GetString("SENDGRID_API_KEY"), fmt.Sprintf("/v3/asm/suppressions/global/%s", email), "https://api.sendgrid.com")
	request.Method = "GET"

	response, err := sendgrid.API(request)
	if err != nil {
		return false, err
	}

	if response.StatusCode >= 300 || response.StatusCode < 200 {
		return false, fmt.Errorf("email unsub addition failed and returned: %+v", response)
	}

	var unsub getGlobalUnsubscriptionSendgridResponse
	err = json.Unmarshal([]byte(response.Body), &unsub)
	if err != nil {
		return false, err
	}

	return unsub.RecipientEmail != "", nil
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
