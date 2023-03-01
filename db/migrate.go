package migrate

import (
	"bufio"
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	pgdriver "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

const sudoFlag = "/* {% require_sudo %} */"

func strToVersion(s string) (uint, error) {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return uint(v), nil
}

func superMigrations(dir string) (map[uint]bool, uint, error) {
	versions := make(map[uint]bool, 0)

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, 0, err
	}

	latestVer := uint(0)

	for _, f := range files {
		name := f.Name()
		if !strings.HasSuffix(name, "up.sql") {
			continue
		}

		fle, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return nil, 0, err
		}
		defer fle.Close()

		scanner := bufio.NewScanner(fle)

		// Only checks the first line of each file
		if scanner.Scan() && scanner.Text() == sudoFlag {
			v, err := strToVersion(strings.Split(name, "_")[0])
			if err != nil {
				return nil, 0, err
			}
			versions[v] = true
			latestVer = v
		}
	}

	return versions, latestVer, nil
}

func currentVersion(m *migrate.Migrate) (uint, error) {
	curVer, dirty, err := m.Version()
	if err != nil && err == migrate.ErrNilVersion {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if dirty {
		return 0, fmt.Errorf("current version %d is dirty, fix before running", curVer)
	}
	return curVer, nil
}

// RunCoreDBMigration should always be used to migrate the core backend database.
// Because the "gallery_migrator" role was introduced in the 56th migration step,
// migrations must be done in two passes (using the default "postgres" role for
// the first 56 migrations, and the "gallery_migrator" role for all subsequent
// migrations).
func RunCoreDBMigration() error {
	coreMigrations := "./db/migrations/core"

	// Migrations up to version 56 should be run with the "postgres" user.
	// Version 56 introduces the "gallery_migrator" role.
	superClient := postgres.NewClient(
		postgres.WithUser(viper.GetString("POSTGRES_SUPERUSER_USER")),
		postgres.WithPassword(viper.GetString("POSTGRES_SUPERUSER_PASSWORD")),
	)
	superMigrate, err := newMigrateInstance(superClient, coreMigrations)
	if err != nil {
		return err
	}
	defer superMigrate.Close()

	curVer, err := currentVersion(superMigrate)
	if err != nil {
		return err
	}

	// migrate updates the version to an older version even if the current
	// version is ahead of it, so we need to manually check before applying
	// the migration
	if curVer < 56 {
		if err := superMigrate.Migrate(56); err != nil {
			return err
		}
		curVer = 56
	}

	// The "gallery_migrator" role should be used for non-privileged changes.
	galleryClient := postgres.NewClient(postgres.WithUser("gallery_migrator"))
	galleryMigrate, err := newMigrateInstance(galleryClient, coreMigrations)
	if err != nil {
		return err
	}
	defer galleryMigrate.Close()

	// Find which migrations need to run as a superuser
	superVersions, lastSuperVer, err := superMigrations(coreMigrations)
	if err != nil {
		return err
	}

	// Apply an up migration if there aren't anymore privileged migrations to run
	if len(superVersions) == 0 || curVer >= lastSuperVer {
		return galleryMigrate.Up()
	}

	superStreak := false
	ver := curVer + 1
	for ; ver <= lastSuperVer; ver++ {
		if !superStreak && superVersions[ver] {
			superStreak = true
			// Skip running the migration if its already applied
			if ver-1 != curVer {
				if err := galleryMigrate.Migrate(ver - 1); err != nil {
					return err
				}
			}
		} else if superStreak && !superVersions[ver] {
			superStreak = false
			if err := superMigrate.Migrate(ver - 1); err != nil {
				return err
			}
		}
	}

	if superStreak {
		if err := superMigrate.Migrate(ver - 1); err != nil {
			return err
		}
	}

	err = galleryMigrate.Up()
	if err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}

func newMigrateInstance(client *sql.DB, dir string) (*migrate.Migrate, error) {
	dir, err := util.FindFile(dir, 3)
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

	superVersions, _, err := superMigrations(dir)
	if err != nil {
		return nil, err
	}

	m.Log = log{superVersions}

	return m, nil
}

type log struct {
	superVersions map[uint]bool
}

func (l log) Printf(format string, v ...any) {
	if len(v) > 0 {
		if parts := strings.Split(v[0].(string), "/"); len(parts) > 0 {
			v, err := strToVersion(parts[0])
			if err != nil {
				panic(err)
			}
			if l.superVersions[v] {
				format = strings.TrimSuffix(format, "\n")
				format += " [super]\n"
			}
		}
	}
	fmt.Fprintf(os.Stderr, format, v...)
}

func (l log) Verbose() bool {
	return false
}
