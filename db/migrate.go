package migrate

import (
	"bufio"
	"database/sql"
	"errors"
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
		if scanner.Scan() && strings.TrimSpace(scanner.Text()) == sudoFlag {
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
	curVer, _, err := m.Version()
	if err != nil && err == migrate.ErrNilVersion {
		return 0, nil
	}
	return curVer, err
}

// SuperUserRequired returns true if the superuser role is needed
// to run migrations based on the database's current state.
func SuperUserRequired(dir string) (bool, error) {
	client, err := postgres.NewClient(postgres.WithUser("gallery_migrator"))
	var errNoRole postgres.ErrRoleDoesNotExist
	if errors.As(err, &errNoRole) {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	migrate, err := newMigrateInstance(client, dir)
	if err != nil {
		return false, err
	}

	curVer, err := currentVersion(migrate)
	if err != nil {
		return false, err
	}

	_, lastSuperVer, err := superMigrations(dir)
	if err != nil {
		return false, err
	}

	return curVer < lastSuperVer, nil
}

// RunMigrations runs unapplied migrations to the database.
// An optional client with can be passed if the current set of
// migrations requires a superuser to run.
func RunMigrations(superClient *sql.DB, dir string) error {
	superRequired, err := SuperUserRequired(dir)
	if err != nil {
		return err
	}

	if superRequired && superClient == nil {
		return errors.New("superuser is required, but client wasn't provided")
	}

	var superMigrate *migrate.Migrate
	var galleryMigrate *migrate.Migrate

	loadMigrate := func() error {
		if galleryMigrate != nil {
			return nil
		}
		galleryClient, err := postgres.NewClient(postgres.WithUser("gallery_migrator"))
		var errNoRole postgres.ErrRoleDoesNotExist
		if errors.As(err, &errNoRole) {
			return nil
		}
		if err != nil {
			return err
		}
		galleryMigrate, err = newMigrateInstance(galleryClient, dir)
		return err
	}

	if superRequired {
		superMigrate, err = newMigrateInstance(superClient, dir)
		if err != nil {
			return err
		}
		defer superMigrate.Close()

		if err := loadMigrate(); err != nil {
			return err
		}
		if galleryMigrate != nil {
			defer galleryMigrate.Close()
		}
	} else {
		// Apply an up migration since a superuser isn't needed
		galleryClient := postgres.MustCreateClient(postgres.WithUser("gallery_migrator"))
		galleryMigrate, err = newMigrateInstance(galleryClient, dir)
		if err != nil {
			return err
		}
		defer galleryMigrate.Close()
		return galleryMigrate.Up()
	}

	var curVer uint

	if galleryMigrate != nil {
		curVer, err = currentVersion(galleryMigrate)
		if err != nil {
			return err
		}
	} else {
		curVer, err = currentVersion(superMigrate)
		if err != nil {
			return err
		}
	}

	// Find which migrations need to run as a superuser
	superVersions, lastSuperVer, err := superMigrations(util.MustFindFile(dir))
	if err != nil {
		return err
	}

	superStreak := false
	ver := curVer + 1
	for ; ver <= lastSuperVer; ver++ {
		if !superStreak && superVersions[ver] {
			superStreak = true
			// Skip running the migration if its the current version already applied
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
			if err := loadMigrate(); err != nil {
				return err
			}
		}
	}

	if superStreak {
		if err := superMigrate.Migrate(ver - 1); err != nil {
			return err
		}
	}

	if galleryMigrate == nil {
		panic("gallery_migrator client never initted!")
	}

	err = galleryMigrate.Up()
	if err == migrate.ErrNoChange {
		return nil
	}

	return err
}

// RunMigration runs all migrations in the specified directory
func RunMigration(client *sql.DB, file string) (*migrate.Migrate, error) {
	m, err := newMigrateInstance(client, file)
	if err != nil {
		return nil, err
	}

	return m, m.Up()
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

	superVersions, _, err := superMigrations(util.MustFindFile(dir))
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
		if msg, ok := v[0].(string); ok {
			if parts := strings.Split(msg, "/"); len(parts) > 0 {
				ver, err := strToVersion(parts[0])
				if err == nil && l.superVersions[ver] {
					format = strings.TrimSuffix(format, "\n")
					format += " [super]\n"
				}
			}
		}
	}
	fmt.Fprintf(os.Stderr, format, v...)
}

func (l log) Verbose() bool {
	return false
}
