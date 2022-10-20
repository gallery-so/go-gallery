package emails

import (
	"bytes"
	"context"
	htmltemplate "html/template"
	"net/http"
	plaintemplate "text/template"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	sendgrid "github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
)

type VerificationEmailInput struct {
	UserID persist.DBID `json:"user_id" binding:"required"`
}

type verificationEmailTemplateData struct {
	Username string
	JWT      string
}

type notificationsEmailTemplateData struct {
	Username      string
	Notifications []string // TODO - this should be a struct with the notification data once that has been merged
}

func sendVerificationEmail(dataloaders *dataloader.Loaders, queries *coredb.Queries, s *sendgrid.Client, htmltemplates *htmltemplate.Template, plaintemplates *plaintemplate.Template) gin.HandlerFunc {

	return func(c *gin.Context) {
		var input VerificationEmailInput
		err := c.ShouldBindJSON(&input)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		user, err := dataloaders.UserByUserID.Load(input.UserID)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		j, err := auth.JWTGeneratePipeline(c, input.UserID)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		plainBuf := new(bytes.Buffer)
		htmlBuf := new(bytes.Buffer)

		plaintemplates.ExecuteTemplate(plainBuf, "verification.txt", verificationEmailTemplateData{
			Username: user.Username.String,
			JWT:      j,
		})

		htmltemplates.ExecuteTemplate(htmlBuf, "verification.gohtml", verificationEmailTemplateData{
			Username: user.Username.String,
			JWT:      j,
		})

		from := mail.NewEmail("Gallery", viper.GetString("FROM_EMAIL"))
		subject := "Gallery Verification"
		to := mail.NewEmail(user.Username.String, user.Email.String)
		plainTextContent := plainBuf.String()
		htmlContent := htmlBuf.String()
		message := mail.NewSingleEmail(from, subject, to, plainTextContent, htmlContent)

		_, err = s.Send(message)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.Status(http.StatusOK)
	}
}

func sendNotificationEmails(queries *coredb.Queries, s *sendgrid.Client, htmltemplates *htmltemplate.Template, plaintemplates *plaintemplate.Template) gin.HandlerFunc {

	return func(c *gin.Context) {
		err := runForUsersWithNotificationsOnForEmailType(c, persist.EmailTypeNotifications, queries, func(u coredb.User) error {

			plainBuf := new(bytes.Buffer)
			htmlBuf := new(bytes.Buffer)

			plaintemplates.ExecuteTemplate(plainBuf, "notifications.txt", notificationsEmailTemplateData{
				Username:      u.Username.String,
				Notifications: []string{"test", "wow", "cool"},
			})

			htmltemplates.ExecuteTemplate(htmlBuf, "notifications.gohtml", notificationsEmailTemplateData{
				Username:      u.Username.String,
				Notifications: []string{"test", "wow", "cool"},
			})

			from := mail.NewEmail("Gallery", viper.GetString("FROM_EMAIL"))
			subject := "Your Gallery Notifications"
			to := mail.NewEmail(u.Username.String, u.Email.String)
			plainTextContent := plainBuf.String()
			htmlContent := htmlBuf.String()

			logger.For(c).Debugf("sending email to %s", u.Email.String)
			logger.For(c).Debugf("html: %s", htmlContent)
			logger.For(c).Debugf("plain: %s", plainTextContent)

			message := mail.NewSingleEmail(from, subject, to, plainTextContent, htmlContent)

			resp, err := s.Send(message)
			if err != nil {
				return err
			}
			logger.For(c).Debugf("email sent: %+v", *resp)
			return nil
		})

		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.Status(http.StatusOK)
	}
}

func runForUsersWithNotificationsOnForEmailType(ctx context.Context, emailType persist.EmailType, queries *coredb.Queries, fn func(u coredb.User) error) error {
	errGroup := new(errgroup.Group)
	var lastID persist.DBID
	var lastCreatedAt time.Time
	var endTime time.Time = time.Now().Add(24 * time.Hour)
	for {
		users, err := queries.GetUsersWithNotificationsOnForEmailType(ctx, coredb.GetUsersWithNotificationsOnForEmailTypeParams{
			Limit:         10000,
			CurAfterTime:  lastCreatedAt,
			CurBeforeTime: endTime,
			CurAfterID:    lastID,
			PagingForward: true,
			Column1:       emailType.String(),
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

		if len(users) < 10000 {
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
