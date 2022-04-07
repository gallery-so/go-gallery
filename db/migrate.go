package migrate

import (
	"database/sql"

	"github.com/golang-migrate/migrate/v4"
	pgdriver "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func RunMigration(migrationDir string, client *sql.DB) error {
	d, err := pgdriver.WithInstance(client, &pgdriver.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithDatabaseInstance("file://"+migrationDir, "postgres", d)
	if err != nil {
		return err
	}
	m.Up()

	return nil
}
