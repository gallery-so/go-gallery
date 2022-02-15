package server

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/asottile/dockerfile"
	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/ory/dockertest"
	"github.com/ory/dockertest/docker"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
)

// N.B. This isn't the entire Docker Compose spec...
type ComposeFile struct {
	Version  string
	Services map[string]Service
}

type Service struct {
	Image       string
	Ports       []string
	Build       map[string]string
	Environment []string
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
	postgres.NewClient().Close()
	return
}

func waitOnCache() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("cache is not available")
		}
	}()
	redis.NewCache(0).Close(false)
	return
}

func loadComposeFile(path string) (f ComposeFile) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}

	err = yaml.Unmarshal(data, &f)
	if err != nil {
		log.Fatal(err)
	}

	return
}

func getImageAndVersion(s string) ([]string, error) {
	imgAndVer := strings.Split(s, ":")
	if len(imgAndVer) != 2 {
		return nil, errors.New("no version specified for image")
	}
	return imgAndVer, nil
}

func getBuildImage(s Service) ([]string, error) {
	res, err := dockerfile.ParseFile(".." + string(filepath.Separator) + s.Build["dockerfile"])
	if err != nil {
		log.Fatal(err)
	}

	for _, cmd := range res {
		if cmd.Cmd == "FROM" {
			return getImageAndVersion(cmd.Value[0])
		}
	}

	return nil, errors.New("no `FROM` directive found in dockerfile")
}

func initPostgres(pool *dockertest.Pool) (*dockertest.Resource, *sql.DB) {
	apps := loadComposeFile("../docker-compose.yml")
	imgAndVer, err := getBuildImage(apps.Services["postgres"])
	if err != nil {
		log.Fatal(err)
	}

	pg, err := pool.RunWithOptions(
		&dockertest.RunOptions{
			Repository: imgAndVer[0],
			Tag:        imgAndVer[1],
			Env:        apps.Services["postgres"].Environment,
		}, configureContainerCleanup,
	)
	if err != nil {
		log.Fatalf("could not start postgres: %s", err)
	}

	// Patch environment to use container
	hostAndPort := strings.Split(pg.GetHostPort("5432/tcp"), ":")
	viper.SetDefault("POSTGRES_HOST", hostAndPort[0])
	viper.SetDefault("POSTGRES_PORT", hostAndPort[1])

	if err = pool.Retry(waitOnDB); err != nil {
		log.Fatalf("could not connect to postgres: %s", err)
	}

	// Seed db
	db = postgres.NewClient()
	for _, f := range []string{"../docker/postgres/init.sql"} {
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
	apps := loadComposeFile("../docker-compose.yml")
	imgAndVer, err := getImageAndVersion(apps.Services["redis"].Image)
	if err != nil {
		log.Fatal(err)
	}

	rd, err := pool.RunWithOptions(
		&dockertest.RunOptions{
			Repository: imgAndVer[0],
			Tag:        imgAndVer[1],
		}, configureContainerCleanup,
	)
	if err != nil {
		log.Fatalf("could not start redis: %s", err)
	}

	// Patch environment to use container
	viper.SetDefault("REDIS_URL", rd.GetHostPort("6379/tcp"))
	if err = pool.Retry(waitOnCache); err != nil {
		log.Fatalf("could not connect to redis: %s", err)
	}

	return rd
}
