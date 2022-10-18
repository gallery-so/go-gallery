package emails

import (
	"net/http"

	"github.com/gammazero/workerpool"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/util"
	sendgrid "github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/spf13/viper"
)

func sendNotificationEmails(loaders *dataloader.Loaders, queries *coredb.Queries, s *sendgrid.Client) gin.HandlerFunc {

	return func(c *gin.Context) {

		wp := workerpool.New(100)

		usersToEmail, err := queries.GetUsersWithNotificationsOn(c) // TODO paginate over every user with notifications on so we don't have this query taking years to run (and have the emails get sent out when each query finishes)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		for _, user := range usersToEmail {
			u := user
			wp.Submit(func() {
				from := mail.NewEmail("TODO", viper.GetString("FROM_EMAIL"))
				subject := "Sending with SendGrid is Fun"
				to := mail.NewEmail("TODO", u.Email.String)
				plainTextContent := "TODO"
				htmlContent := "<strong>TODO</strong>"
				message := mail.NewSingleEmail(from, subject, to, plainTextContent, htmlContent)

				_, err := s.Send(message)
				if err != nil {
					util.ErrResponse(c, http.StatusInternalServerError, err)
					return
				}
			})
		}

		wp.StopWait()

		c.Status(http.StatusOK)
	}
}
