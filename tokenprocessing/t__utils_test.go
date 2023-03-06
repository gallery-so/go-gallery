package tokenprocessing

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/jackc/pgx/v4/pgxpool"
	migrate "github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/docker"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/stretchr/testify/assert"
)

var (
	ensAddress = "0x57f1887a8bf19b14fc0df6fd9b2acc9af147ea85"
)

func setupTest(t *testing.T) (*assert.Assertions, *sql.DB, *pgxpool.Pool) {
	setDefaults()

	r, err := docker.StartPostgres()
	if err != nil {
		t.Fatal(err)
	}

	hostAndPort := strings.Split(r.GetHostPort("5432/tcp"), ":")
	t.Setenv("POSTGRES_HOST", hostAndPort[0])
	t.Setenv("POSTGRES_PORT", hostAndPort[1])

	db := postgres.MustCreateClient()
	pgx := postgres.NewPgxClient()
	migrate, err := migrate.RunMigration(db, "./db/migrations/core")
	if err != nil {
		t.Fatalf("failed to seed db: %s", err)
	}
	t.Cleanup(func() {
		migrate.Close()
		r.Close()
	})

	return assert.New(t), db, pgx
}
