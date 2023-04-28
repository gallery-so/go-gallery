package pushnotifications

import (
	"context"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/pushnotifications/expo"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"net/http"
	"os"
)

func init() {
	env.RegisterValidation("EXPO_PUSH_API_URL", "required")
	env.RegisterValidation("PUSH_NOTIFICATIONS_SECRET", "required")
}

func InitServer() {
	setDefaults()
	router := coreInitServer()
	logger.For(nil).Info("Starting push notifications server...")
	http.Handle("/", router)
}

func coreInitServer() *gin.Engine {
	initSentry()
	logger.InitWithGCPDefaults()

	if env.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	http.DefaultClient = &http.Client{Transport: tracing.NewTracingTransport(http.DefaultTransport, false)}

	router := gin.Default()
	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.ErrLogger())
	router.GET("/alive", util.HealthCheckHandler())

	// Return 200 on auth failures to prevent task/job retries
	authOpts := middleware.BasicAuthOptionBuilder{}
	basicAuthHandler := middleware.BasicHeaderAuthRequired(env.GetString("PUSH_NOTIFICATIONS_SECRET"), authOpts.WithFailureStatus(http.StatusOK))

	taskGroup := router.Group("/tasks", basicAuthHandler, middleware.TaskRequired())
	jobGroup := router.Group("/jobs", basicAuthHandler)

	ctx := context.Background()

	pgx := postgres.NewPgxClient()
	queries := db.New(pgx)

	logger.For(ctx).Info("Registering handlers...")

	apiURL := env.GetString("EXPO_PUSH_API_URL")
	accessToken := env.GetString("EXPO_PUSH_ACCESS_TOKEN")
	expoHandler := expo.NewPushNotificationHandler(ctx, queries, apiURL, accessToken)

	taskGroup.POST("send-push-notification", sendPushNotificationHandler(expoHandler))
	jobGroup.POST("check-push-tickets", checkPushTicketsHandler(expoHandler))

	return router
}

func sendPushNotificationHandler(expoHandler *expo.PushNotificationHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		message := task.PushNotificationMessage{}

		if err := c.ShouldBindJSON(&message); err != nil {
			// Send StatusOK here to indicate that the task should NOT be retried. If the task data can't
			// be mapped to our message type, there's nothing a retry can do to resolve it.
			logger.For(c).Error("sendPushNotificationHandler failed to bind JSON to message struct")
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		err := expoHandler.SendPushNotification(message.PushTokenID, message.Title, message.Subtitle, message.Body, message.Data, message.Sound, message.Badge)

		if err != nil {
			logger.For(c).WithError(err).WithField("pushTokenID", message.PushTokenID).Warn("failed to send push notification")

			// Don't retry on ErrDeviceNotRegistered or ErrPushTokenNotFound. The Expo handler
			// will take care of removing unregistered tokens from the database when ErrDeviceNotRegistered
			// occurs, and no special handling is required for ErrPushTokenNotFound (token was probably
			// just unregistered at some point)

			var responseCode int
			if err == expo.ErrDeviceNotRegistered || err == expo.ErrPushTokenNotFound {
				// A 2xx response code won't trigger a task retry
				responseCode = http.StatusOK
			} else {
				// A non-2xx response code will trigger a task retry
				responseCode = http.StatusInternalServerError
			}

			util.ErrResponse(c, responseCode, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func checkPushTicketsHandler(expoHandler *expo.PushNotificationHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		err := expoHandler.CheckPushTickets()
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("VERSION", "")
	viper.SetDefault("PUSH_NOTIFICATIONS_SECRET", "push-notifications-secret")
	viper.SetDefault("EXPO_PUSH_ACCESS_TOKEN", "")
	viper.SetDefault("EXPO_PUSH_API_URL", "https://exp.host/--/api/v2/push")

	viper.AutomaticEnv()

	if env.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
	} else {
		fi := "local"
		if len(os.Args) > 1 {
			fi = os.Args[1]
		}
		envFile := util.ResolveEnvFile("pushnotifications", fi)
		util.LoadEncryptedEnvFile(envFile)
	}

	if env.GetString("ENV") != "local" {
		util.VarNotSetTo("SENTRY_DSN", "")
		util.VarNotSetTo("VERSION", "")
		util.VarNotSetTo("PUSH_NOTIFICATIONS_SECRET", "push-notifications-secret")
	}
}

func initSentry() {
	if env.GetString("ENV") == "local" {
		logger.For(nil).Info("skipping sentry init")
		return
	}

	logger.For(nil).Info("initializing sentry...")

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              env.GetString("SENTRY_DSN"),
		Environment:      env.GetString("ENV"),
		TracesSampleRate: env.GetFloat64("SENTRY_TRACES_SAMPLE_RATE"),
		Release:          env.GetString("VERSION"),
		AttachStacktrace: true,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			event = auth.ScrubEventCookies(event, hint)
			event = sentryutil.UpdateErrorFingerprints(event, hint)
			event = sentryutil.UpdateLogErrorEvent(event, hint)
			return event
		},
	})

	if err != nil {
		logger.For(nil).Fatalf("failed to start sentry: %s", err)
	}
}
