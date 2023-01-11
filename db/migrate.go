package migrate

import (
	"database/sql"

	"github.com/golang-migrate/migrate/v4"
	pgdriver "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/mikeydub/go-gallery/util"
)

func RunMigration(client *sql.DB, file string) error {
	dir, err := util.FindFile(file, 3)
	if err != nil {
		return err
	}

	d, err := pgdriver.WithInstance(client, &pgdriver.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithDatabaseInstance("file://"+dir, "postgres", d)
	if err != nil {
		return err
	}

	err = m.Up()
	if err != nil {
		return err
	}

	return nil
}
