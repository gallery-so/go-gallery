package main

import (
	"database/sql"
	"fmt"
	"os"

	migrate "github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh/terminal"
)

func init() {
	viper.SetDefault("POSTGRES_MIGRATION_USER", "")
	viper.SetDefault("POSTGRES_MIGRATION_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("POSTGRES_HOST", "localhost")
	viper.SetDefault("POSTGRES_PORT", "")
	viper.AutomaticEnv()
}

func main() {
	coreMigrations := "./db/migrations/core"

	superRequired, err := migrate.SuperUserRequired(coreMigrations)
	if err != nil {
		panic(err)
	}

	var superClient *sql.DB

	if superRequired {
		var user string
		fmt.Print("Username to use for privileged migrations: ")
		fmt.Scanln(&user)

		fmt.Printf("Password for %s: ", user)
		pw, err := terminal.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			panic(err)
		}

		superClient = postgres.MustCreateClient(
			postgres.WithUser(user),
			postgres.WithPassword(string(pw)),
		)
	}

	if err := migrate.RunMigrations(superClient, coreMigrations); err != nil {
		fmt.Fprint(os.Stderr, err)
	}
}
