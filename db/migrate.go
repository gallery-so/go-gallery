package migrate

import (
	"database/sql"
	"github.com/golang-migrate/migrate/v4"
	pgdriver "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
)

// RunCoreDBMigration should always be used to migrate the core backend database.
// Because the "gallery_migrator" role was introduced in the 56th migration step,
// migrations must be done in two passes (using the default "postgres" role for
// the first 56 migrations, and the "gallery_migrator" role for all subsequent
// migrations).
func RunCoreDBMigration() error {
	coreMigrations := "./db/migrations/core"

	// Migrations up to version 56 should be run with the "postgres" user.
	// Version 56 introduces the "gallery_migrator" role.
	client := postgres.NewClient(postgres.WithUser("postgres"))

	m, err := RunMigrationToVersion(client, coreMigrations, 56)
	if err != nil {
		return err
	}

	m.Close()

	// The "gallery_migrator" role should be used for all future migrations.
	client = postgres.NewClient(postgres.WithUser("gallery_migrator"))
	m, err = RunMigration(client, coreMigrations)

	// Ignore ErrNoChange here, since that will happen until we add a 57th migration
	if err != nil && err != migrate.ErrNoChange {
		return err
	}

	m.Close()
	return nil
}

// RunMigration runs all migrations in the specified directory
func RunMigration(client *sql.DB, file string) (*migrate.Migrate, error) {
	m, err := newMigrateInstance(client, file)
	if err != nil {
		return nil, err
	}

	return m, m.Up()
}

// RunMigrationToVersion runs migrations in the specified directory, up to (and including) the
// specified migration version number
func RunMigrationToVersion(client *sql.DB, file string, toVersion uint) (*migrate.Migrate, error) {
	m, err := newMigrateInstance(client, file)
	if err != nil {
		return nil, err
	}

	return m, m.Migrate(toVersion)
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
