package emails

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/gin-gonic/gin"
	"github.com/sendgrid/rest"
	sendgrid "github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"golang.org/x/sync/errgroup"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/emails"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/notifications"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/store"
	"github.com/mikeydub/go-gallery/util"
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

	gid := env.GetString("SENDGRID_UNSUBSCRIBE_NOTIFICATIONS_GROUP_ID")
	gidInt, err := strconv.Atoi(gid)
	if err != nil {
		panic(err)
	}

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

		if _, err := sendNotificationEmailToUser(c, userWithPII, input.ToEmail, gidInt, queries, s, 10, 5, input.SendRealEmails); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.Status(http.StatusOK)
	}
}

func sendNotificationEmails(queries *coredb.Queries, s *sendgrid.Client, r *redis.Cache) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		l, _ := r.Get(ctx, "send-notification-emails")
		if len(l) > 0 {
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

		// kick off an event that will have notification handlers that will fan out the notification to all users
		err = event.Dispatch(c, coredb.Event{
			ID:             persist.GenerateID(),
			ResourceTypeID: persist.ResourceTypeAllUsers,
			Action:         persist.ActionAnnouncement,
			UserID:         galleryUser.ID,
			SubjectID:      galleryUser.ID,
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

	gid := env.GetString("SENDGRID_UNSUBSCRIBE_NOTIFICATIONS_GROUP_ID")
	gidInt, err := strconv.Atoi(gid)
	if err != nil {
		return err
	}

	emailsSent := new(atomic.Uint64)
	defer func() {
		logger.For(c).Infof("sent %d emails", emailsSent.Load())
	}()
	return runForUsersWithNotificationsOnForEmailType(c, persist.EmailTypeNotifications, queries, func(u coredb.PiiUserView) error {

		response, err := sendNotificationEmailToUser(c, u, u.PiiEmailAddress, gidInt, queries, s, 10, 5, sendRealEmails)
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

func sendNotificationEmailToUser(c context.Context, u coredb.PiiUserView, emailRecipient persist.Email, unsubscribeGroupID int, queries *coredb.Queries, s *sendgrid.Client, searchLimit int32, resultLimit int, sendRealEmail bool) (*rest.Response, error) {

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
		asm := mail.NewASM()
		asm.SetGroupID(unsubscribeGroupID)
		asm.AddGroupsToDisplay(unsubscribeGroupID)
		m.SetASM(asm)

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
	DigestValues     DigestValues `json:"digest_values"`
	Username         string       `json:"username"`
	UnsubscribeToken string       `json:"unsubscribe_token"`
}

func sendDigestEmails(queries *coredb.Queries, s *sendgrid.Client, r *redis.Cache, b *store.BucketStorer, gql *graphql.Client) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if env.GetString("ENV") == "production" {
			l, _ := r.Get(ctx, "send-digest-emails")
			if len(l) > 0 {
				logger.For(ctx).Infof("digest emails already being sent")
				return
			}
			r.Set(ctx, "send-digest-emails", []byte("sending"), 1*time.Hour)
		}

		vals, err := buildDigestTemplate(ctx, b, queries, gql)
		if err != nil {
			logger.For(ctx).Errorf("error getting digest values: %s", err)
			util.ErrResponse(ctx, http.StatusInternalServerError, err)
			return
		}

		err = sendDigestEmailsToAllUsers(ctx, vals, queries, s)
		if err != nil {
			logger.For(ctx).Errorf("error sending notification emails: %s", err)
			util.ErrResponse(ctx, http.StatusInternalServerError, err)
			return
		}

		// wipe the overrides so that the overrides can only be used once
		byt, _ := json.Marshal(DigestValueOverrides{})

		_, err = b.Write(ctx, overrideFile, byt)
		if err != nil {
			util.ErrResponse(ctx, http.StatusInternalServerError, fmt.Errorf("failed to clear overrides: %s", err))
			return
		}

		ctx.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

// sendDigestTestEmail sends a digest email to an admin user's email address.
func sendDigestTestEmail(q *coredb.Queries, s *sendgrid.Client, b *store.BucketStorer, gql *graphql.Client) gin.HandlerFunc {

	gid := env.GetString("SENDGRID_UNSUBSCRIBE_DIGEST_GROUP_ID")
	gidInt, err := strconv.Atoi(gid)
	if err != nil {
		panic(err)
	}

	// Ideally this is handled via OAuth in Retool
	return func(ctx *gin.Context) {
		var input struct {
			Email string `json:"email" binding:"required"`
		}

		if err := ctx.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(ctx, http.StatusBadRequest, err)
			return
		}

		if !strings.HasSuffix(input.Email, "gallery.so") {
			util.ErrResponse(ctx, http.StatusBadRequest, fmt.Errorf("must be a gallery.so email"))
			return
		}

		user, err := q.GetUserByVerifiedEmailAddress(ctx, input.Email)
		if err != nil {
			util.ErrResponse(ctx, http.StatusNotFound, err)
			return
		}

		roles, err := auth.RolesByUserID(ctx, q, user.ID)
		if err != nil {
			util.ErrResponse(ctx, http.StatusInternalServerError, err)
			return
		}

		_, isAdmin := util.FindFirst(roles, func(r persist.Role) bool { return r == persist.RoleAdmin })
		if !isAdmin {
			util.ErrResponse(ctx, http.StatusUnauthorized, fmt.Errorf("%s is not an admin", user.Username.String))
			return
		}

		userWithPII, err := q.GetUserWithPIIByID(ctx, user.ID)
		if err != nil {
			util.ErrResponse(ctx, http.StatusBadRequest, err)
			return
		}

		if input.Email != userWithPII.PiiEmailAddress.String() {
			util.ErrResponse(ctx, http.StatusBadRequest, errEmailMismatch{userID: user.ID})
			return
		}

		template, err := buildDigestTemplate(ctx, b, q, gql)
		if err != nil {
			util.ErrResponse(ctx, http.StatusInternalServerError, err)
			return
		}

		r, err := sendDigestEmailToUser(ctx, userWithPII, userWithPII.PiiEmailAddress, gidInt, template, s)
		if err != nil {
			util.ErrResponse(ctx, http.StatusInternalServerError, err)
			return
		}

		if r.StatusCode != http.StatusAccepted {
			util.ErrResponse(ctx, http.StatusInternalServerError, fmt.Errorf("failed to send email: %s; statusCode: %d", r.Body, r.StatusCode))
			return
		}

		ctx.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func sendDigestEmailsToAllUsers(c context.Context, v DigestValues, queries *coredb.Queries, s *sendgrid.Client) error {

	gid := env.GetString("SENDGRID_UNSUBSCRIBE_DIGEST_GROUP_ID")
	gidInt, err := strconv.Atoi(gid)
	if err != nil {
		panic(err)
	}

	logger.For(c).Infof("sending digest emails to all users with values: %+v", v)
	emailsSent := new(atomic.Uint64)
	defer func() {
		logger.For(c).Infof("sent %d emails", emailsSent.Load())
	}()
	return runForUsersWithNotificationsOnForEmailType(c, persist.EmailTypeDigest, queries, func(u coredb.PiiUserView) error {

		response, err := sendDigestEmailToUser(c, u, u.PiiEmailAddress, gidInt, v, s)
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

func sendDigestEmailToUser(c context.Context, u coredb.PiiUserView, emailRecipient persist.Email, unsubscribeGroupID int, digestValues DigestValues, s *sendgrid.Client) (*rest.Response, error) {
	j, err := auth.GenerateEmailVerificationToken(c, u.ID, u.PiiEmailAddress.String())
	if err != nil {
		return nil, fmt.Errorf("failed to generate jwt for user %s: %w", u.ID, err)
	}

	data := digestEmailDynamicTemplateData{
		DigestValues:     digestValues,
		Username:         u.Username.String,
		UnsubscribeToken: j,
	}

	if data.DigestValues.IntroText == nil || *data.DigestValues.IntroText == "" {
		data.DigestValues.IntroText = util.ToPointer(defaultIntroText(u.Username.String))
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

	// send email
	from := mail.NewEmail("Gallery", env.GetString("FROM_EMAIL"))
	to := mail.NewEmail(u.Username.String, emailRecipient.String())
	m := mail.NewV3Mail()
	m.SetFrom(from)
	p := mail.NewPersonalization()
	m.SetTemplateID(env.GetString("SENDGRID_DIGEST_TEMPLATE_ID"))
	p.DynamicTemplateData = asMap
	m.AddPersonalizations(p)
	m.AddCategories("weekly_digest")
	p.AddTos(to)
	asm := mail.NewASM()
	asm.SetGroupID(unsubscribeGroupID)
	asm.AddGroupsToDisplay(unsubscribeGroupID)
	m.SetASM(asm)

	response, err := s.Send(m)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func runForUsersWithNotificationsOnForEmailType(ctx context.Context, emailType persist.EmailType, queries *coredb.Queries, fn func(u coredb.PiiUserView) error) error {
	errGroup := new(errgroup.Group)
	var lastID persist.DBID
	var lastCreatedAt time.Time
	// end time seemingly ensures that we don't send emails to users who signed up after the last time we ran this function, but assumes this function could take a day to run.
	// This could probably just be set to time.Now() and it would be fine but if it ain't broke don't fix it
	var endTime time.Time = time.Now().Add(24 * time.Hour)
	requiredStatus := persist.EmailVerificationStatusVerified
	if isDevEnv() {
		// if we are not in production, the only users returned will be those with email status admin verified
		requiredStatus = persist.EmailVerificationStatusAdmin
	}

	for {
		// paginate emailsAtATime users at a time, running fn on each asynchronously
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
				return fn(u)
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
