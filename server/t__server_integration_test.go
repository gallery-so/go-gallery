package server

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/ory/dockertest"
	"github.com/ory/dockertest/docker"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
)

type UserTestSuite struct {
	suite.Suite
	apiVersion int
	resources  []*dockertest.Resource
	pool       *dockertest.Pool
	tc         *TestConfig
	db         *sql.DB
}

func configureWithCleanup(config *docker.HostConfig) {
	config.AutoRemove = true
	config.RestartPolicy = docker.RestartPolicy{Name: "no"}
}

func waitOnDB() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("db is not available")
		}
	}()
	postgres.NewClient().Close()
	return
}

func (s *UserTestSuite) SetupSuite() {
	setDefaults()
	// create docker client
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("could not connect to docker: %s", err)
	}

	// create a new db instance
	resource, err := pool.RunWithOptions(
		&dockertest.RunOptions{
			Repository: "postgres",
			Tag:        "14",
			Env: []string{
				"POSTGRES_HOST_AUTH_METHOD=trust",
				"POSTGRES_USER=postgres",
				"POSTGRES_PORT=5432",
				"POSTGRES_DB=postgres",
			},
		}, configureWithCleanup,
	)
	if err != nil {
		log.Fatalf("could not start postgres: %s", err)
	}

	hostAndPort := strings.Split(resource.GetHostPort("5432/tcp"), ":")
	viper.SetDefault("POSTGRES_HOST", hostAndPort[0])
	viper.SetDefault("POSTGRES_PORT", hostAndPort[1])

	if err = pool.Retry(waitOnDB); err != nil {
		log.Fatalf("could not connect to postgres: %s", err)
	}

	s.pool = pool
	s.db = postgres.NewClient()
	s.resources = append(s.resources, resource)
	s.tc = initializeTestServer(s.db, s.Assertions, s.apiVersion)
}

func (s *UserTestSuite) TearDownSuite() {
	for _, r := range s.resources {
		if err := s.pool.Purge(r); err != nil {
			log.Fatalf("could not purge resource: %s", err)
		}
	}
}

func (s *UserTestSuite) TestExample() {
	s.Equal(5, 5)
}

func TestUserTestSuite(t *testing.T) {
	suite.Run(t, new(UserTestSuite))
}
