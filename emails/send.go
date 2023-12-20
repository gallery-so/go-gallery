package emails

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/publicapi"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/emails"
	"github.com/mikeydub/go-gallery/service/notifications"
	"github.com/mikeydub/go-gallery/service/redis"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sendgrid/rest"
	sendgrid "github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"golang.org/x/sync/errgroup"
)

func init() {
	env.RegisterValidation("FROM_EMAIL", "required", "email")
	env.RegisterValidation("SENDGRID_VERIFICATION_TEMPLATE_ID", "required")
	env.RegisterValidation("PUBSUB_NOTIFICATIONS_EMAILS_SUBSCRIPTION", "required")
	env.RegisterValidation("PUBSUB_DIGEST_EMAILS_SUBSCRIPTION", "required")

}

const emailsAtATime = 10_000

type sendNotificationEmailHttpInput struct {
	UserID         persist.DBID  `json:"user_id" binding:"required"`
	ToEmail        persist.Email `json:"to_email" binding:"required"`
	SendRealEmails bool          `json:"send_real_emails"`
}

type errNoEmailSet struct {
	userID persist.DBID
}

type errEmailMismatch struct {
	userID persist.DBID
}

func sendVerificationEmail(dataloaders *dataloader.Loaders, queries *coredb.Queries, s *sendgrid.Client) gin.HandlerFunc {

	return func(c *gin.Context) {
		var input emails.VerificationEmailInput
		err := c.ShouldBindJSON(&input)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userWithPII, err := queries.GetUserWithPIIByID(c, input.UserID)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		if userWithPII.PiiEmailAddress.String() == "" {
			util.ErrResponse(c, http.StatusBadRequest, errNoEmailSet{userID: input.UserID})
			return
		}

		emailAddress := userWithPII.PiiEmailAddress.String()
		j, err := auth.GenerateEmailVerificationToken(c, input.UserID, emailAddress)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		//logger.For(c).Debugf("sending verification email to %s with token %s", emailAddress, j)

		from := mail.NewEmail("Gallery", env.GetString("FROM_EMAIL"))
		to := mail.NewEmail(userWithPII.Username.String, emailAddress)
		m := mail.NewV3Mail()
		m.SetFrom(from)
		p := mail.NewPersonalization()
		m.SetTemplateID(env.GetString("SENDGRID_VERIFICATION_TEMPLATE_ID"))
		p.DynamicTemplateData = map[string]interface{}{
			"username":          userWithPII.Username.String,
			"verificationToken": j,
		}
		m.AddPersonalizations(p)
		p.AddTos(to)

		_, err = s.Send(m)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		//logger.For(c).Debugf("email sent: %+v", *response)

		c.Status(http.StatusOK)
	}
}

type notificationsEmailDynamicTemplateData struct {
	Notifications    []notifications.UserFacingNotificationData `json:"notifications"`
	Username         string                                     `json:"username"`
	UnsubscribeToken string                                     `json:"unsubscribeToken"`
}

func adminSendNotificationEmail(queries *coredb.Queries, s *sendgrid.Client) gin.HandlerFunc {

	return func(c *gin.Context) {

		var input sendNotificationEmailHttpInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		userWithPII, err := queries.GetUserWithPIIByID(c, input.UserID)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		if _, err := sendNotificationEmailToUser(c, userWithPII, input.ToEmail, queries, s, 10, 5, input.SendRealEmails); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.Status(http.StatusOK)
	}
}

func sendNotificationEmails(queries *coredb.Queries, s *sendgrid.Client, r *redis.Cache) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		l, _ := r.Get(ctx, "send-notification-emails")
		if l != nil && len(l) > 0 {
			logger.For(ctx).Infof("notification emails already being sent")
			return
		}
		r.Set(ctx, "send-notification-emails", []byte("sending"), 1*time.Hour)
		err := sendNotificationEmailsToAllUsers(ctx, queries, s, env.GetString("ENV") == "production")
		if err != nil {
			logger.For(ctx).Errorf("error sending notification emails: %s", err)
			return
		}
	}
}

func sendAnnouncementNotification(q *coredb.Queries) gin.HandlerFunc {
	galleryUser, err := q.GetUserByUsername(context.Background(), "gallery")
	if err != nil {
		panic(err)
	}
	return func(c *gin.Context) {
		var in persist.AnnouncementDetails
		err := c.ShouldBindJSON(&in)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		err = event.Dispatch(c, coredb.Event{
			ID:             persist.GenerateID(),
			ResourceTypeID: persist.ResourceTypeAllUsers,
			Action:         persist.ActionAnnouncement,
			UserID:         galleryUser.ID,
			ActorID:        persist.DBIDToNullStr(galleryUser.ID),
			Data: persist.EventData{
				AnnouncementDetails: &in,
			},
		})
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func sendNotificationEmailsToAllUsers(c context.Context, queries *coredb.Queries, s *sendgrid.Client, sendRealEmails bool) error {

	emailsSent := new(atomic.Uint64)
	defer func() {
		logger.For(c).Infof("sent %d emails", emailsSent.Load())
	}()
	return runForUsersWithNotificationsOnForEmailType(c, persist.EmailTypeNotifications, queries, func(u coredb.PiiUserView) error {

		response, err := sendNotificationEmailToUser(c, u, u.PiiEmailAddress, queries, s, 10, 5, sendRealEmails)
		if err != nil {
			return err
		}
		if response != nil {
			if response.StatusCode >= 299 || response.StatusCode < 200 {
				logger.For(c).Errorf("error sending email to %s: %s", u.Username.String, response.Body)
			} else {
				emailsSent.Add(1)
			}
		}

		return nil
	})
}

func sendNotificationEmailToUser(c context.Context, u coredb.PiiUserView, emailRecipient persist.Email, queries *coredb.Queries, s *sendgrid.Client, searchLimit int32, resultLimit int, sendRealEmail bool) (*rest.Response, error) {

	// generate notification data for user
	notifs, err := queries.GetRecentUnseenNotifications(c, coredb.GetRecentUnseenNotificationsParams{
		OwnerID:      u.ID,
		Lim:          searchLimit,
		CreatedAfter: time.Now().Add(-7 * 24 * time.Hour),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get notifications for user %s: %w", u.ID, err)
	}

	j, err := auth.GenerateEmailVerificationToken(c, u.ID, u.PiiEmailAddress.String())
	if err != nil {
		return nil, fmt.Errorf("failed to generate jwt for user %s: %w", u.ID, err)
	}

	data := notificationsEmailDynamicTemplateData{
		Notifications:    make([]notifications.UserFacingNotificationData, 0, resultLimit),
		Username:         u.Username.String,
		UnsubscribeToken: j,
	}
	notifTemplates := make(chan notifications.UserFacingNotificationData)
	errChan := make(chan error)

	for _, n := range notifs {
		notif := n
		go func() {
			// notifTemplate, err := notifToTemplateData(c, queries, notif)
			notifTemplate, err := notifications.NotificationToUserFacingData(c, queries, notif)
			if err != nil {
				errChan <- err
				return
			}
			notifTemplates <- notifTemplate
		}()
	}

outer:
	for i := 0; i < len(notifs); i++ {
		select {
		case err := <-errChan:
			logger.For(c).Errorf("failed to get notification template data: %v", err)
		case notifTemplate := <-notifTemplates:
			data.Notifications = append(data.Notifications, notifTemplate)
			if len(data.Notifications) >= resultLimit {
				break outer
			}
		}
	}

	if len(data.Notifications) == 0 {
		return nil, nil
	}

	asJSON, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	asMap := make(map[string]interface{})

	err = json.Unmarshal(asJSON, &asMap)
	if err != nil {
		return nil, err
	}

	if sendRealEmail {
		// send email
		from := mail.NewEmail("Gallery", env.GetString("FROM_EMAIL"))
		to := mail.NewEmail(u.Username.String, emailRecipient.String())
		m := mail.NewV3Mail()
		m.SetFrom(from)
		p := mail.NewPersonalization()
		m.SetTemplateID(env.GetString("SENDGRID_NOTIFICATIONS_TEMPLATE_ID"))
		p.DynamicTemplateData = asMap
		m.AddPersonalizations(p)
		p.AddTos(to)

		response, err := s.Send(m)
		if err != nil {
			return nil, err
		}
		return response, nil
	}

	logger.For(c).Infof("would have sent email to %s (username: %s): %s", u.ID, u.Username.String, string(asJSON))

	return &rest.Response{StatusCode: 200, Body: "not sending real emails", Headers: map[string][]string{}}, nil
}

type digestEmailDynamicTemplateData struct {
	DigestValues     DigestValues `json:"digestValues"`
	Username         string       `json:"username"`
	UnsubscribeToken string       `json:"unsubscribeToken"`
}

func sendDigestEmails(queries *coredb.Queries, loaders *dataloader.Loaders, s *sendgrid.Client, r *redis.Cache, stg *storage.Client, fapi *publicapi.FeedAPI) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// mimic backend auth
		ctx.Set("auth.user_id", persist.DBID(""))
		ctx.Set("auth.auth_error", nil)

		l, _ := r.Get(ctx, "send-digest-emails")
		if l != nil && len(l) > 0 {
			logger.For(ctx).Infof("digest emails already being sent")
			return
		}
		r.Set(ctx, "send-digest-emails", []byte("sending"), 1*time.Hour)
		vals, err := getDigest(ctx, stg, fapi, queries, loaders)
		if err != nil {
			logger.For(ctx).Errorf("error getting digest values: %s", err)
			return
		}
		err = sendDigestEmailsToAllUsers(ctx, vals, queries, s, env.GetString("ENV") == "production")
		if err != nil {
			logger.For(ctx).Errorf("error sending notification emails: %s", err)
			return
		}
	}
}

func sendDigestEmailsToAllUsers(c context.Context, v DigestValues, queries *coredb.Queries, s *sendgrid.Client, sendRealEmails bool) error {

	emailsSent := new(atomic.Uint64)
	defer func() {
		logger.For(c).Infof("sent %d emails", emailsSent.Load())
	}()
	return runForUsersWithNotificationsOnForEmailType(c, persist.EmailTypeDigest, queries, func(u coredb.PiiUserView) error {

		response, err := sendDigestEmailToUser(c, u, u.PiiEmailAddress, v, s, sendRealEmails)
		if err != nil {
			return err
		}
		if response != nil {
			if response.StatusCode >= 299 || response.StatusCode < 200 {
				logger.For(c).Errorf("error sending email to %s: %s", u.Username.String, response.Body)
			} else {
				emailsSent.Add(1)
			}
		}

		return nil
	})
}

func sendDigestEmailToUser(c context.Context, u coredb.PiiUserView, emailRecipient persist.Email, digestValues DigestValues, s *sendgrid.Client, sendRealEmail bool) (*rest.Response, error) {

	j, err := auth.GenerateEmailVerificationToken(c, u.ID, u.PiiEmailAddress.String())
	if err != nil {
		return nil, fmt.Errorf("failed to generate jwt for user %s: %w", u.ID, err)
	}

	data := digestEmailDynamicTemplateData{
		DigestValues:     digestValues,
		Username:         u.Username.String,
		UnsubscribeToken: j,
	}

	asJSON, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	asMap := make(map[string]interface{})

	err = json.Unmarshal(asJSON, &asMap)
	if err != nil {
		return nil, err
	}

	if sendRealEmail {
		// send email
		from := mail.NewEmail("Gallery", env.GetString("FROM_EMAIL"))
		to := mail.NewEmail(u.Username.String, emailRecipient.String())
		m := mail.NewV3Mail()
		m.SetFrom(from)
		p := mail.NewPersonalization()
		m.SetTemplateID(env.GetString("SENDGRID_DIGEST_TEMPLATE_ID"))
		p.DynamicTemplateData = asMap
		m.AddPersonalizations(p)
		p.AddTos(to)

		response, err := s.Send(m)
		if err != nil {
			return nil, err
		}
		return response, nil
	}

	logger.For(c).Infof("would have sent email to %s (username: %s): %s", u.ID, u.Username.String, string(asJSON))

	return &rest.Response{StatusCode: 200, Body: "not sending real emails", Headers: map[string][]string{}}, nil
}

func runForUsersWithNotificationsOnForEmailType(ctx context.Context, emailType persist.EmailType, queries *coredb.Queries, fn func(u coredb.PiiUserView) error) error {
	errGroup := new(errgroup.Group)
	var lastID persist.DBID
	var lastCreatedAt time.Time
	var endTime time.Time = time.Now().Add(24 * time.Hour)
	requiredStatus := persist.EmailVerificationStatusVerified
	if isDevEnv() {
		requiredStatus = persist.EmailVerificationStatusAdmin
	}

	for {
		users, err := queries.GetUsersWithEmailNotificationsOnForEmailType(ctx, coredb.GetUsersWithEmailNotificationsOnForEmailTypeParams{
			Limit:               emailsAtATime,
			CurAfterTime:        lastCreatedAt,
			CurBeforeTime:       endTime,
			CurAfterID:          lastID,
			PagingForward:       true,
			EmailVerified:       requiredStatus,
			EmailUnsubscription: emailType.String(),
		})
		if err != nil {
			return err
		}

		for _, user := range users {
			u := user
			errGroup.Go(func() error {
				err = fn(u)
				if err != nil {
					return err
				}
				return nil
			})
		}

		if len(users) < emailsAtATime {
			break
		}

		if len(users) > 0 {
			lastUser := users[len(users)-1]
			lastID = lastUser.ID
			lastCreatedAt = lastUser.CreatedAt
		}
	}

	return errGroup.Wait()
}

func (e errNoEmailSet) Error() string {
	return fmt.Sprintf("user %s has no email", e.userID)
}

func (e errEmailMismatch) Error() string {
	return fmt.Sprintf("wrong email for user %s", e.userID)
}
