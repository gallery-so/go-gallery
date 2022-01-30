package server

import (
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/ory/dockertest"
	"github.com/ory/dockertest/docker"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
)

type UserTestSuite struct {
	suite.Suite
	version   int
	resources []*dockertest.Resource
	pool      *dockertest.Pool
	tc        *TestConfig
	db        *sql.DB
}

func configureContainerCleanup(config *docker.HostConfig) {
	config.AutoRemove = true
	config.RestartPolicy = docker.RestartPolicy{Name: "no"}
}

func waitOnDB() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("db is not available")
		}
	}()

	postgres.NewClient()
	return
}

func waitOnCache() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("cache is not available")
		}
	}()

	redis.NewCache(0)
	return
}

func initPostgres(pool *dockertest.Pool) (*dockertest.Resource, *sql.DB) {
	pg, err := pool.RunWithOptions(
		&dockertest.RunOptions{
			Repository: "postgres",
			Tag:        "14",
			Env: []string{
				"POSTGRES_HOST_AUTH_METHOD=trust",
				"POSTGRES_USER=postgres",
				"POSTGRES_PORT=5432",
				"POSTGRES_DB=postgres",
			}}, configureContainerCleanup,
	)
	if err != nil {
		log.Fatalf("could not start postgres: %s", err)
	}

	hostAndPort := strings.Split(pg.GetHostPort("5432/tcp"), ":")
	viper.SetDefault("POSTGRES_HOST", hostAndPort[0])
	viper.SetDefault("POSTGRES_PORT", hostAndPort[1])

	if err = pool.Retry(waitOnDB); err != nil {
		log.Fatalf("could not connect to postgres: %s", err)
	}

	db = postgres.NewClient()
	for _, f := range []string{"../scripts/initial_setup.sql", "../scripts/post_import.sql"} {
		migration, err := os.ReadFile(f)

		if err != nil {
			panic(err)
		}

		_, err = db.Exec(string(migration))
		if err != nil {
			log.Fatalf("failed to seed the db: %s", err)
		}
	}

	return pg, db
}

func initRedis(pool *dockertest.Pool) *dockertest.Resource {
	rd, err := pool.RunWithOptions(
		&dockertest.RunOptions{Repository: "redis", Tag: "6"}, configureContainerCleanup,
	)
	if err != nil {
		log.Fatalf("could not start redis: %s", err)
	}

	viper.SetDefault("REDIS_URL", rd.GetHostPort("6379/tcp"))
	if err = pool.Retry(waitOnCache); err != nil {
		log.Fatalf("could not connect to redis: %s", err)
	}

	return rd
}

func (s *UserTestSuite) SetupTest() {
	setDefaults()

	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("could not connect to docker: %s", err)
	}
	pg, pgClient := initPostgres(pool)
	rd := initRedis(pool)

	s.pool = pool
	s.db = pgClient
	s.resources = append(s.resources, pg, rd)
	s.tc = initializeTestServer(s.db, s.Assertions, s.version)
}

func (s *UserTestSuite) TearDownTest() {
	for _, r := range s.resources {
		if err := s.pool.Purge(r); err != nil {
			log.Fatalf("could not purge resource: %s", err)
		}
	}
}

func (s *UserTestSuite) TestUserJourney() {
	s.Equal(1, 1)
}

func TestUserTestSuite(t *testing.T) {
	suite.Run(t, new(UserTestSuite))
}
