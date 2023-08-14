// Updates the recommendation matrices stored in GCP Cloud Storage.
package main

import (
	"context"
	"time"

	sentrypkg "github.com/getsentry/sentry-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/recommend/userpref"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/store"
	"github.com/mikeydub/go-gallery/util"
)

var envVar string

var rootCmd = &cobra.Command{
	Use:   "upload [BUCKET] [OBJECT]",
	Short: "Save matrices to GCP Cloud Storage",
	Long:  "Save matrices to GCP Cloud Storage",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		now := time.Now()

		if env.GetString("ENV") == "local" && envVar != "local" {
			util.LoadEncryptedEnvFile(util.ResolveEnvFile("userpref-upload", envVar))
		}

		logger.For(ctx).Infof("uploading matrices to %s/%s", args[0], args[1])

		n, err := upload(ctx, args[0], args[1])

		if err != nil {
			logger.For(ctx).Errorf("failed to write to %s: %s", args[1], err)
			sentryutil.ReportError(ctx, err)
			return
		}

		logger.For(ctx).Infof("wrote %d bytes to %s in %s", n, args[1], time.Since(now))
	},
}

func main() {
	rootCmd.Flags().StringVarP(&envVar, "env", "e", "local", "source env to pull")
	rootCmd.Execute()
}

func init() {
	setDefaults()
	logger.InitWithGCPDefaults()
	initSentry()
}

func upload(ctx context.Context, bucketName, objectName string) (int, error) {
	s := rpc.NewStorageClient(ctx)
	b := store.NewBucketStorer(s, bucketName)
	pgx := postgres.NewPgxClient()
	q := db.New(pgx)

	m := userpref.ReadMatrices(ctx, q)
	byt, err := m.MarshalBinary()
	if err != nil {
		return 0, err
	}

	return b.WriteGzip(ctx, objectName, byt)
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("SENTRY_TRACES_SAMPLE_RATE", "1.0")
	viper.SetDefault("GAE_VERSION", "")
	viper.AutomaticEnv()
}

func initSentry() {
	if env.GetString("ENV") == "local" {
		logger.For(nil).Info("skipping sentry init")
		return
	}

	logger.For(nil).Info("initializing sentry...")

	err := sentrypkg.Init(sentrypkg.ClientOptions{
		MaxSpans:         100000,
		Dsn:              env.GetString("SENTRY_DSN"),
		Environment:      env.GetString("ENV"),
		TracesSampleRate: env.GetFloat64("SENTRY_TRACES_SAMPLE_RATE"),
		Release:          env.GetString("GAE_VERSION"),
		AttachStacktrace: true,
	})

	if err != nil {
		logger.For(nil).Fatalf("failed to start sentry: %s", err)
	}
}
