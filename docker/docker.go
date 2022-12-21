package docker

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/asottile/dockerfile"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/util"
	"github.com/ory/dockertest"
	"github.com/ory/dockertest/docker"
	"gopkg.in/yaml.v2"
)

func StartPostgres() (resource *dockertest.Resource, err error) {
	pool, err := newPool(time.Minute * 3)
	if err != nil {
		return nil, err
	}

	r, err := startService(pool, "postgres")
	if err != nil {
		return nil, err
	}

	hostAndPort := strings.Split(r.GetHostPort("5432/tcp"), ":")
	host := hostAndPort[0]
	port := hostAndPort[1]

	if err = pool.Retry(waitOnDB(host, port, "postgres", "", "postgres")); err != nil {
		log.Fatalf("could not connect to postgres: %s", err)
	}

	return r, nil
}

func StartPostgresIndexer() (resource *dockertest.Resource, err error) {
	pool, err := newPool(time.Minute * 3)
	if err != nil {
		return nil, err
	}

	r, err := startService(pool, "postgres_indexer")
	if err != nil {
		return nil, err
	}

	hostAndPort := strings.Split(r.GetHostPort("5432/tcp"), ":")
	host := hostAndPort[0]
	port := hostAndPort[1]

	if err = pool.Retry(waitOnDB(host, port, "postgres", "", "postgres")); err != nil {
		log.Fatalf("could not connect to postgres: %s", err)
	}

	return r, nil
}

func StartRedis() (*dockertest.Resource, error) {
	pool, err := newPool(time.Minute * 3)
	if err != nil {
		return nil, err
	}

	r, err := startService(pool, "redis")
	if err != nil {
		return nil, err
	}

	hostAndPort := r.GetHostPort("6379/tcp")

	if err = pool.Retry(waitOnCache(hostAndPort)); err != nil {
		log.Fatalf("could not connect to redis: %s", err)
	}

	return r, nil
}

// N.B. This isn't the entire Docker Compose spec...
type compose struct {
	Version  string             `yaml:"version"`
	Services map[string]service `yaml:"services"`
}

type service struct {
	Image       string                 `yaml:"image"`
	Ports       []string               `yaml:"ports"`
	Build       map[string]interface{} `yaml:"build"`
	Environment []string               `yaml:"environment"`
	Command     string                 `yaml:"command"`
}

func startService(pool *dockertest.Pool, service string) (*dockertest.Resource, error) {
	apps, err := loadComposeFile()
	if err != nil {
		return nil, err
	}

	serviceConf, ok := apps.Services[service]
	if !ok {
		return nil, fmt.Errorf("service=%s not configured in docker-compose.yml", service)
	}

	img, tag, err := baseImage(serviceConf)
	if err != nil {
		return nil, err
	}

	return pool.RunWithOptions(
		&dockertest.RunOptions{
			Repository: img,
			Tag:        tag,
			Env:        serviceConf.Environment,
		},
		func(c *docker.HostConfig) {
			c.AutoRemove = true
			c.RestartPolicy = docker.RestartPolicy{Name: "no"}
		},
	)
}

func newPool(waitTime time.Duration) (*dockertest.Pool, error) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		return nil, err
	}
	pool.MaxWait = waitTime
	return pool, nil
}

func waitOnDB(host, port, user, password, db string) func() error {
	return func() error {
		db, err := sql.Open("pgx", fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s", host, port, user, password, db))
		if err != nil {
			return err
		}
		defer db.Close()
		return db.Ping()
	}
}

func waitOnCache(url string) func() error {
	return func() error {
		return redis.NewCache(0).Close(false)
	}
}

func loadComposeFile() (compose, error) {
	path := util.MustFindFile("./docker-compose.yml")

	data, err := os.ReadFile(path)
	if err != nil {
		return compose{}, err
	}

	var c compose
	err = yaml.Unmarshal(data, &c)

	return c, err
}

func imageAndTag(s string) (string, string, error) {
	uri := strings.Split(s, ":")
	if len(uri) != 2 {
		return uri[0], "latest", nil
	}
	return uri[0], uri[1], nil
}

func baseImage(s service) (string, string, error) {
	path := util.MustFindFile("./docker-compose.yml")

	dockerPath := filepath.Join(filepath.Dir(path), s.Build["dockerfile"].(string))
	absPath, _ := filepath.Abs(dockerPath)
	res, err := dockerfile.ParseFile(absPath)
	if err != nil {
		return "", "", err
	}

	for _, cmd := range res {
		if cmd.Cmd == "FROM" {
			return imageAndTag(cmd.Value[0])
		}
	}

	return "", "", errors.New("no `FROM` directive found in dockerfile")
}
