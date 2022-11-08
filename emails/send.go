package emails

import (
	"context"
	"fmt"
	"net/http"
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

const emailsAtATime = 10_000

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

type errNoEmailSet struct {
	userID persist.DBID
}

func sendVerificationEmail(dataloaders *dataloader.Loaders, queries *coredb.Queries, s *sendgrid.Client) gin.HandlerFunc {

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

		if user.Email == "" {
			util.ErrResponse(c, http.StatusBadRequest, errNoEmailSet{userID: input.UserID})
			return
		}

		j, err := auth.JWTGeneratePipeline(c, input.UserID)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		from := mail.NewEmail("Gallery", viper.GetString("FROM_EMAIL"))
		to := mail.NewEmail(user.Username.String, user.Email.String())
		m := mail.NewV3Mail()
		m.SetFrom(from)
		p := mail.NewPersonalization()
		m.SetTemplateID(viper.GetString("SENDGRID_VERIFICATION_TEMPLATE_ID"))
		p.DynamicTemplateData = map[string]interface{}{
			"jwt": j,
		}
		m.AddPersonalizations(p)
		p.AddTos(to)

		response, err := s.Send(m)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		logger.For(c).Debugf("email sent: %+v", *response)

		c.Status(http.StatusOK)
	}
}

func sendNotificationEmails(queries *coredb.Queries, s *sendgrid.Client) gin.HandlerFunc {

	return func(c *gin.Context) {
		err := runForUsersWithNotificationsOnForEmailType(c, persist.EmailTypeNotifications, queries, func(u coredb.User) error {

			from := mail.NewEmail("Gallery", viper.GetString("FROM_EMAIL"))
			to := mail.NewEmail(u.Username.String, u.Email.String())
			m := mail.NewV3Mail()
			m.SetFrom(from)
			p := mail.NewPersonalization()
			m.SetTemplateID(viper.GetString("SENDGRID_NOTIFICATIONS_TEMPLATE_ID"))
			p.DynamicTemplateData = map[string]interface{}{
				"notifications": []string{"notification 1", "notification 2"},
			}
			m.Asm.GroupID = viper.GetInt("SENDGRID_UNSUBSCRIBE_NOTIFICATIONS_GROUP_ID")
			m.AddPersonalizations(p)
			p.AddTos(to)

			response, err := s.Send(m)
			if err != nil {
				return err
			}
			logger.For(c).Debugf("email sent: %+v", *response)
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
		users, err := queries.GetUsersWithEmailNotificationsOnForEmailType(ctx, coredb.GetUsersWithEmailNotificationsOnForEmailTypeParams{
			Limit:         emailsAtATime,
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
