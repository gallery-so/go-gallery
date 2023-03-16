package emails

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"cloud.google.com/go/pubsub"
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
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
)

func init() {
	env.RegisterEnvValidation("FROM_EMAIL", []string{"required", "email"})
	env.RegisterEnvValidation("SENDGRID_VERIFICATION_TEMPLATE_ID", []string{"required"})
	env.RegisterEnvValidation("PUBSUB_NOTIFICATIONS_EMAILS_SUBSCRIPTION", []string{"required"})
}

const emailsAtATime = 10_000

type VerificationEmailInput struct {
	UserID persist.DBID `json:"user_id" binding:"required"`
}

type sendNotificationEmailHttpInput struct {
	UserID         persist.DBID  `json:"user_id,required"`
	ToEmail        persist.Email `json:"to_email,required"`
	SendRealEmails bool          `json:"send_real_emails"`
}

type verificationEmailTemplateData struct {
	Username string
	JWT      string
}

type errNoEmailSet struct {
	userID persist.DBID
}

type errEmailMismatch struct {
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
		j, err := jwtGenerate(input.UserID, emailAddress)
		if err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		//logger.For(c).Debugf("sending verification email to %s with token %s", emailAddress, j)

		from := mail.NewEmail("Gallery", viper.GetString("FROM_EMAIL"))
		to := mail.NewEmail(userWithPII.Username.String, emailAddress)
		m := mail.NewV3Mail()
		m.SetFrom(from)
		p := mail.NewPersonalization()
		m.SetTemplateID(viper.GetString("SENDGRID_VERIFICATION_TEMPLATE_ID"))
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

type notificationEmailDynamicTemplateData struct {
	Actor          string       `json:"actor"`
	Action         string       `json:"action"`
	CollectionName string       `json:"collectionName"`
	CollectionID   persist.DBID `json:"collectionId"`
	PreviewText    string       `json:"previewText"`
}
type notificationsEmailDynamicTemplateData struct {
	Notifications    []notificationEmailDynamicTemplateData `json:"notifications"`
	Username         string                                 `json:"username"`
	UnsubscribeToken string                                 `json:"unsubscribeToken"`
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

func autoSendNotificationEmails(queries *coredb.Queries, s *sendgrid.Client, psub *pubsub.Client) error {
	sub := psub.Subscription(viper.GetString("PUBSUB_NOTIFICATIONS_EMAILS_SUBSCRIPTION"))

	ctx := context.Background()
	return sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		err := sendNotificationEmailsToAllUsers(ctx, queries, s, viper.GetString("ENV") == "production")
		if err != nil {
			logger.For(ctx).Errorf("error sending notification emails: %s", err)
			msg.Nack()
			return
		}
		msg.Ack()
	})
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

	j, err := jwtGenerate(u.ID, u.PiiEmailAddress.String())
	if err != nil {
		return nil, fmt.Errorf("failed to generate jwt for user %s: %w", u.ID, err)
	}

	data := notificationsEmailDynamicTemplateData{
		Notifications:    make([]notificationEmailDynamicTemplateData, 0, resultLimit),
		Username:         u.Username.String,
		UnsubscribeToken: j,
	}
	notifTemplates := make(chan notificationEmailDynamicTemplateData)
	errChan := make(chan error)

	for _, n := range notifs {
		notif := n
		go func() {
			notifTemplate, err := notifToTemplateData(c, queries, notif)
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
		from := mail.NewEmail("Gallery", viper.GetString("FROM_EMAIL"))
		to := mail.NewEmail(u.Username.String, emailRecipient.String())
		m := mail.NewV3Mail()
		m.SetFrom(from)
		p := mail.NewPersonalization()
		m.SetTemplateID(viper.GetString("SENDGRID_NOTIFICATIONS_TEMPLATE_ID"))
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

func notifToTemplateData(ctx context.Context, queries *coredb.Queries, n coredb.Notification) (notificationEmailDynamicTemplateData, error) {

	switch n.Action {
	case persist.ActionAdmiredFeedEvent:
		feedEvent, err := queries.GetFeedEventByID(ctx, n.FeedEventID)
		if err != nil {
			return notificationEmailDynamicTemplateData{}, fmt.Errorf("failed to get feed event for admire %s: %w", n.FeedEventID, err)
		}
		collection, _ := queries.GetCollectionById(ctx, feedEvent.Data.CollectionID)
		data := notificationEmailDynamicTemplateData{}
		if collection.ID != "" && collection.Name.String != "" {
			data.CollectionID = collection.ID
			data.CollectionName = collection.Name.String
			data.Action = "admired your additions to"
		} else {
			data.Action = "admired your gallery update"
		}
		if len(n.Data.AdmirerIDs) > 1 {
			data.Actor = fmt.Sprintf("%d collectors", len(n.Data.AdmirerIDs))
		} else {
			actorUser, err := queries.GetUserById(ctx, n.Data.AdmirerIDs[0])
			if err != nil {
				return notificationEmailDynamicTemplateData{}, err
			}
			data.Actor = actorUser.Username.String
		}
		return data, nil
	case persist.ActionUserFollowedUsers:
		if len(n.Data.FollowerIDs) > 1 {
			return notificationEmailDynamicTemplateData{
				Actor:  fmt.Sprintf("%d users", len(n.Data.FollowerIDs)),
				Action: "followed you",
			}, nil
		}
		if len(n.Data.FollowerIDs) == 1 {
			userActor, err := queries.GetUserById(ctx, n.Data.FollowerIDs[0])
			if err != nil {
				return notificationEmailDynamicTemplateData{}, fmt.Errorf("failed to get user for follower %s: %w", n.Data.FollowerIDs[0], err)
			}
			action := "followed you"
			if n.Data.FollowedBack {
				action = "followed you back"
			}
			return notificationEmailDynamicTemplateData{
				Actor:  userActor.Username.String,
				Action: action,
			}, nil
		}
		return notificationEmailDynamicTemplateData{}, fmt.Errorf("no follower ids")
	case persist.ActionCommentedOnFeedEvent:
		comment, err := queries.GetCommentByCommentID(ctx, n.CommentID)
		if err != nil {
			return notificationEmailDynamicTemplateData{}, fmt.Errorf("failed to get comment for comment %s: %w", n.CommentID, err)
		}
		userActor, err := queries.GetUserById(ctx, comment.ActorID)
		if err != nil {
			return notificationEmailDynamicTemplateData{}, fmt.Errorf("failed to get user for comment actor %s: %w", comment.ActorID, err)
		}
		feedEvent, err := queries.GetFeedEventByID(ctx, n.FeedEventID)
		if err != nil {
			return notificationEmailDynamicTemplateData{}, fmt.Errorf("failed to get feed event for comment %s: %w", n.FeedEventID, err)
		}
		collection, _ := queries.GetCollectionById(ctx, feedEvent.Data.CollectionID)
		if collection.ID != "" {
			return notificationEmailDynamicTemplateData{
				Actor:          userActor.Username.String,
				Action:         "commented on your additions to",
				CollectionName: collection.Name.String,
				CollectionID:   collection.ID,
				PreviewText:    util.TruncateWithEllipsis(comment.Comment, 20),
			}, nil
		}
		return notificationEmailDynamicTemplateData{
			Actor:       userActor.Username.String,
			Action:      "commented on your gallery update",
			PreviewText: util.TruncateWithEllipsis(comment.Comment, 20),
		}, nil
	case persist.ActionViewedGallery:
		if len(n.Data.AuthedViewerIDs)+len(n.Data.UnauthedViewerIDs) > 1 {
			return notificationEmailDynamicTemplateData{
				Actor:  fmt.Sprintf("%d collectors", len(n.Data.AuthedViewerIDs)+len(n.Data.UnauthedViewerIDs)),
				Action: "viewed your gallery",
			}, nil
		}
		if len(n.Data.AuthedViewerIDs) == 1 {
			userActor, err := queries.GetUserById(ctx, n.Data.AuthedViewerIDs[0])
			if err != nil {
				return notificationEmailDynamicTemplateData{}, fmt.Errorf("failed to get user for viewer %s: %w", n.Data.AuthedViewerIDs[0], err)
			}
			return notificationEmailDynamicTemplateData{
				Actor:  userActor.Username.String,
				Action: "viewed your gallery",
			}, nil
		}
		if len(n.Data.UnauthedViewerIDs) == 1 {
			return notificationEmailDynamicTemplateData{
				Actor:  "Someone",
				Action: "viewed your gallery",
			}, nil
		}

		return notificationEmailDynamicTemplateData{}, fmt.Errorf("no viewer ids")
	default:
		return notificationEmailDynamicTemplateData{}, fmt.Errorf("unknown action %s", n.Action)
	}
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
