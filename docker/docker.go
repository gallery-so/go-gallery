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
	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/ory/dockertest"
	"github.com/ory/dockertest/docker"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
)

// N.B. This isn't the entire Docker Compose spec...
type ComposeFile struct {
	Version  string             `yaml:"version"`
	Services map[string]Service `yaml:"services"`
}

type Service struct {
	Image       string                 `yaml:"image"`
	Ports       []string               `yaml:"ports"`
	Build       map[string]interface{} `yaml:"build"`
	Environment []string               `yaml:"environment"`
	Command     string                 `yaml:"command"`
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

	db, err := sql.Open(
		"pgx",
		fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s",
			viper.GetString("POSTGRES_HOST"),
			viper.GetInt("POSTGRES_PORT"),
			viper.GetString("POSTGRES_USER"),
			viper.GetString("POSTGRES_PASSWORD"),
			viper.GetString("POSTGRES_DB"),
		),
	)
	if err != nil {
		panic(err)
	}

	if err := db.Ping(); err != nil {
		panic(err)
	}

	db.Close()
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

func getBuildImage(path string, s Service) ([]string, error) {
	dockerPath := filepath.Join(filepath.Dir(path), s.Build["dockerfile"].(string))
	absPath, _ := filepath.Abs(dockerPath)
	res, err := dockerfile.ParseFile(absPath)
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

func InitPostgres(composePath string) *dockertest.Resource {
	pool, err := dockertest.NewPool("")
	pool.MaxWait = 3 * time.Minute
	if err != nil {
		log.Fatalf("could not connect to docker: %s", err)
	}

	absPath, _ := filepath.Abs(composePath)
	apps := loadComposeFile(absPath)
	imgAndVer, err := getBuildImage(absPath, apps.Services["postgres"])
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
	viper.Set("POSTGRES_USER", "postgres")
	viper.Set("POSTGRES_PASSWORD", "")
	viper.Set("POSTGRES_DB", "postgres")
	viper.Set("ENV", "local")

	if err = pool.Retry(waitOnDB); err != nil {
		log.Fatalf("could not connect to postgres: %s", err)
	}

	return pg
}

func InitRedis(composePath string) *dockertest.Resource {
	pool, err := dockertest.NewPool("")
	pool.MaxWait = 3 * time.Minute
	if err != nil {
		log.Fatalf("could not connect to docker: %s", err)
	}

	apps := loadComposeFile(composePath)
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
