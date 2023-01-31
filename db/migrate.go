package migrate

import (
	"database/sql"

	"github.com/golang-migrate/migrate/v4"
	pgdriver "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/mikeydub/go-gallery/util"
)

// RunMigration runs all migrations in the specified directory
func RunMigration(client *sql.DB, file string) error {
	m, err := newMigrateInstance(client, file)
	if err != nil {
		return err
	}

	err = m.Up()
	if err != nil {
		return err
	}

	return nil
}

// RunMigrationToVersion runs migrations in the specified directory, up to (and including) the
// specified migration version number
func RunMigrationToVersion(client *sql.DB, file string, toVersion uint) error {
	m, err := newMigrateInstance(client, file)
	if err != nil {
		return err
	}

	err = m.Migrate(toVersion)
	if err != nil {
		return err
	}

	return nil
}

func newMigrateInstance(client *sql.DB, file string) (*migrate.Migrate, error) {
	dir, err := util.FindFile(file, 3)
	if err != nil {
		return nil, err
	}

	d, err := pgdriver.WithInstance(client, &pgdriver.Config{})
	if err != nil {
		return nil, err
	}

	m, err := migrate.NewWithDatabaseInstance("file://"+dir, "postgres", d)
	if err != nil {
		return nil, err
	}

	return m, nil
}
