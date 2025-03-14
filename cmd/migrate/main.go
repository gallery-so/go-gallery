package main

import (
	"database/sql"
	"fmt"
	"os"

	migrate "github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

func init() {
	viper.SetDefault("POSTGRES_USER", "gallery_migrator")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("POSTGRES_HOST", "localhost")
	viper.SetDefault("POSTGRES_PORT", "")
	viper.AutomaticEnv()
}

func main() {
	migrations := "./db/migrations/core"

	superRequired, err := migrate.SuperUserRequired(migrations)
	if err != nil {
		panic(err)
	}

	var superClient *sql.DB

	if superRequired {
		var user string
		fmt.Print("Username to use for privileged migrations: ")
		fmt.Scanln(&user)

		fmt.Printf("Password for %s: ", user)
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			panic(err)
		}

		fmt.Println("\nAttempting to connect...")

		superClient = postgres.MustCreateClient(
			postgres.WithUser(user),
			postgres.WithPassword(string(pw)),
		)
	}

	if err := migrate.RunMigrations(superClient, migrations); err != nil {
		fmt.Fprint(os.Stderr, err)
	}
}
