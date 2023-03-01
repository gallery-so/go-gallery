package main

import (
	"fmt"
	"os"

	migrate "github.com/mikeydub/go-gallery/db"
	"github.com/spf13/viper"
)

func init() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_MIGRATION_USER", "")
	viper.SetDefault("POSTGRES_MIGRATION_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("POSTGRES_HOST", "localhost")
	viper.SetDefault("POSTGRES_PORT", "")
	viper.SetDefault("POSTGRES_SUPERUSER_USER", "postgres")
	viper.SetDefault("POSTGRES_SUPERUSER_PASSWORD", "")
	viper.AutomaticEnv()
}

func main() {
	if err := migrate.RunCoreDBMigration(); err != nil {
		fmt.Fprint(os.Stderr, err)
	}
}
