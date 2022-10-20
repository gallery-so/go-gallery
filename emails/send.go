package emails

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	sendgrid "github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
)

func sendNotificationEmails(loaders *dataloader.Loaders, queries *coredb.Queries, s *sendgrid.Client) gin.HandlerFunc {

	return func(c *gin.Context) {
		err := runForUsersWithNotificationsOn(c, persist.EmailTypeNotifications, queries, func(u coredb.User) error {
			from := mail.NewEmail("TODO", viper.GetString("FROM_EMAIL"))
			subject := "Sending with SendGrid is Fun"
			to := mail.NewEmail("TODO", u.Email.String)
			plainTextContent := "TODO"
			htmlContent := "<strong>TODO</strong>"
			message := mail.NewSingleEmail(from, subject, to, plainTextContent, htmlContent)

			_, err := s.Send(message)
			if err != nil {
				return err
			}
			return err
		})

		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.Status(http.StatusOK)
	}
}

func runForUsersWithNotificationsOn(ctx context.Context, emailType persist.EmailType, queries *coredb.Queries, fn func(u coredb.User) error) error {
	errGroup := new(errgroup.Group)
	var lastID persist.DBID
	var lastCreatedAt time.Time
	for {
		users, err := queries.GetUsersWithNotificationsOn(ctx, coredb.GetUsersWithNotificationsOnParams{
			Limit:         10000,
			CurAfterTime:  lastCreatedAt,
			CurAfterID:    lastID,
			PagingForward: true,
			EmailType:     int32(emailType),
		})
		if err != nil {
			return err
		}

		errGroup.Go(func() error {
			for _, user := range users {
				err = fn(user)
				if err != nil {
					return err
				}
			}
			return nil
		})

		if len(users) < 10000 {
			break
		}
	}

	return errGroup.Wait()
}
