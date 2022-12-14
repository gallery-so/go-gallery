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
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/util"
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

func loadComposeFile() (f ComposeFile) {
	path, err := util.FindFile("./docker-compose.yml", 3)
	if err != nil {
		log.Fatal(err)
	}

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
	path, err := util.FindFile("./docker-compose.yml", 3)
	if err != nil {
		log.Fatal(err)
	}

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

func InitPostgres() (resource *dockertest.Resource, callback func()) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("could not connect to docker: %s", err)
	}
	pool.MaxWait = 3 * time.Minute

	logger.For(nil).Info("starting postgres")

	apps := loadComposeFile()
	imgAndVer, err := getBuildImage(apps.Services["postgres"])
	if err != nil {
		log.Fatal(err)
	}

	logger.For(nil).Info("building postgres image")

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
	logger.For(nil).Info("postgres started")

	// Patch environment to use container
	pgHost := viper.GetString("POSTGRES_HOST")
	pgPort := viper.GetString("POSTGRES_PORT")
	pgUser := viper.GetString("POSTGRES_USER")
	pgPass := viper.GetString("POSTGRES_PASSWORD")
	pgDb := viper.GetString("POSTGRES_DB")
	env := viper.GetString("ENV")

	hostAndPort := strings.Split(pg.GetHostPort("5432/tcp"), ":")
	viper.Set("POSTGRES_HOST", hostAndPort[0])
	viper.Set("POSTGRES_PORT", hostAndPort[1])
	viper.Set("POSTGRES_USER", "postgres")
	viper.Set("POSTGRES_PASSWORD", "")
	viper.Set("POSTGRES_DB", "postgres")
	viper.Set("ENV", "local")

	// Called to restore original environment
	callback = func() {
		viper.Set("POSTGRES_HOST", pgHost)
		viper.Set("POSTGRES_PORT", pgPort)
		viper.Set("POSTGRES_USER", pgUser)
		viper.Set("POSTGRES_PASSWORD", pgPass)
		viper.Set("POSTGRES_DB", pgDb)
		viper.Set("ENV", env)
	}

	if err = pool.Retry(waitOnDB); err != nil {
		log.Fatalf("could not connect to postgres: %s", err)
	}

	return pg, callback
}

func InitPostgresIndexer() (resource *dockertest.Resource, callback func()) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("could not connect to docker: %s", err)
	}
	pool.MaxWait = 3 * time.Minute

	apps := loadComposeFile()
	imgAndVer, err := getBuildImage(apps.Services["postgres_indexer"])
	if err != nil {
		log.Fatal(err)
	}

	pg, err := pool.RunWithOptions(
		&dockertest.RunOptions{
			Repository: imgAndVer[0],
			Tag:        imgAndVer[1],
			Env:        apps.Services["postgres_indexer"].Environment,
		}, configureContainerCleanup,
	)
	if err != nil {
		log.Fatalf("could not start postgres: %s", err)
	}

	// Patch environment to use container
	pgHost := viper.GetString("POSTGRES_HOST")
	pgPort := viper.GetString("POSTGRES_PORT")
	pgUser := viper.GetString("POSTGRES_USER")
	pgPass := viper.GetString("POSTGRES_PASSWORD")
	pgDb := viper.GetString("POSTGRES_DB")
	env := viper.GetString("ENV")

	hostAndPort := strings.Split(pg.GetHostPort("5432/tcp"), ":")
	fmt.Println(hostAndPort)
	viper.Set("POSTGRES_HOST", hostAndPort[0])
	viper.Set("POSTGRES_PORT", hostAndPort[1])
	viper.Set("POSTGRES_USER", "postgres")
	viper.Set("POSTGRES_PASSWORD", "")
	viper.Set("POSTGRES_DB", "postgres")
	viper.Set("ENV", "local")

	// Called to restore original environment
	callback = func() {
		viper.Set("POSTGRES_HOST", pgHost)
		viper.Set("POSTGRES_PORT", pgPort)
		viper.Set("POSTGRES_USER", pgUser)
		viper.Set("POSTGRES_PASSWORD", pgPass)
		viper.Set("POSTGRES_DB", pgDb)
		viper.Set("ENV", env)
	}

	if err = pool.Retry(waitOnDB); err != nil {
		log.Fatalf("could not connect to postgres: %s", err)
	}

	return pg, callback
}

func InitRedis() (resource *dockertest.Resource, callback func()) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("could not connect to docker: %s", err)
	}
	pool.MaxWait = 3 * time.Minute

	apps := loadComposeFile()
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
	url := viper.Get("REDIS_URL")
	viper.Set("REDIS_URL", rd.GetHostPort("6379/tcp"))

	// Called to restore original environment
	callback = func() {
		viper.Set("REDIS_URL", url)
	}

	if err = pool.Retry(waitOnCache); err != nil {
		log.Fatalf("could not connect to redis: %s", err)
	}

	return rd, callback
}
