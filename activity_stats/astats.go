package activitystats

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
	"google.golang.org/api/idtoken"
)

// InitServer initializes the autosocial server
func InitServer() {
	setDefaults()
	ctx := context.Background()
	router := CoreInitServer(ctx)

	logger.For(nil).Info("Starting autosocial server...")
	http.Handle("/", router)
}

func CoreInitServer(ctx context.Context) *gin.Engine {
	InitSentry()
	logger.InitWithGCPDefaults()

	http.DefaultClient = &http.Client{Transport: tracing.NewTracingTransport(http.DefaultTransport, false)}

	router := gin.Default()
	pgx := postgres.NewPgxClient()
	queries := coredb.New(pgx)
	stg := rpc.NewStorageClient(ctx)

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())
	router.POST("/calculate_activity_badges", cloudSchedulerMiddleware, calculateTop100ActivityBadges(queries, stg, pgx))
	router.POST("/update_top_100_conf", retoolMiddleware, updateTop100ActivityConfiguration(stg))
	router.GET("/get_top_100_conf", retoolMiddleware, getTop100ActivityConfiguration(stg))

	return router
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("GAE_VERSION", "")
	viper.SetDefault("GOOGLE_CLOUD_PROJECT", "gallery-dev-322005")
	viper.SetDefault("CONFIGURATION_BUCKET", "gallery-dev-configurations")
	viper.SetDefault("SCHEDULER_AUDIENCE", "")

	viper.AutomaticEnv()

	if env.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
	} else {
		fi := "local"
		if len(os.Args) > 1 {
			fi = os.Args[1]
		}
		envFile := util.ResolveEnvFile("activitystats", fi)
		util.LoadEncryptedEnvFile(envFile)
	}

	if env.GetString("ENV") != "local" {
		util.VarNotSetTo("SENTRY_DSN", "")
	}
}

func InitSentry() {
	if env.GetString("ENV") == "local" {
		logger.For(nil).Info("skipping sentry init")
		return
	}

	logger.For(nil).Info("initializing sentry...")

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              env.GetString("SENTRY_DSN"),
		Environment:      env.GetString("ENV"),
		TracesSampleRate: env.GetFloat64("SENTRY_TRACES_SAMPLE_RATE"),
		Release:          env.GetString("GAE_VERSION"),
		AttachStacktrace: true,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			event = auth.ScrubEventCookies(event, hint)
			return event
		},
	})

	if err != nil {
		logger.For(nil).Fatalf("failed to start sentry: %s", err)
	}
}

func retoolMiddleware(ctx *gin.Context) {
	if err := auth.RetoolAuthorized(ctx); err != nil {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	ctx.Next()
}

func cloudSchedulerMiddleware(c *gin.Context) {

	idToken := c.GetHeader("Authorization")
	if idToken == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "No ID token provided"})
		return
	}

	_, err := validateIDToken(c, idToken)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid ID token"})
		return
	}

	c.Next()
}

func validateIDToken(ctx context.Context, idToken string) (*idtoken.Payload, error) {

	idToken = strings.TrimPrefix(idToken, "Bearer ")

	validator, err := idtoken.NewValidator(ctx)
	if err != nil {
		logger.For(nil).Errorf("error creating id token validator: %s", err)
		return nil, err
	}

	payload, err := validator.Validate(ctx, idToken, env.GetString("SCHEDULER_AUDIENCE"))
	if err != nil {
		logger.For(nil).Errorf("error validating id token: %s", err)
		return nil, err
	}

	serviceEmail, err := getServiceAccountEmail()
	if err != nil {
		logger.For(nil).Errorf("error getting service account email: %s", err)
		return nil, err
	}

	if payload.Claims["email"] != serviceEmail {
		logger.For(nil).Errorf("id token email does not match service account email: %s", err)
		return nil, err
	}

	return payload, nil
}

func getServiceAccountEmail() (string, error) {
	const url = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/email"

	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Metadata-Flavor", "Google")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
