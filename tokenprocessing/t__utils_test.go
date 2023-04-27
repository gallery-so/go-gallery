package tokenprocessing

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/jackc/pgx/v4/pgxpool"
	migrate "github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/docker"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/option"
)

var (
	testBlockFrom            = 0
	testBlockTo              = 100
	testAddress              = "0x9a3f9764b21adaf3c6fdf6f947e6d3340a3f8ac5"
	galleryMembershipAddress = "0xe01569ca9b39e55bc7c0dfa09f05fa15cb4c7698"
	ensAddress               = "0x57f1887a8bf19b14fc0df6fd9b2acc9af147ea85"
	contribAddress           = "0xda3845b44736b57e05ee80fc011a52a9c777423a" // Jarrel's address with a contributor card in it
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

func newStorageClient(ctx context.Context) *storage.Client {
	stg, err := storage.NewClient(ctx, option.WithCredentialsJSON(util.LoadEncryptedServiceKey("secrets/dev/service-key-dev.json")))
	if err != nil {
		panic(err)
	}
	return stg
}
