package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/jackc/pgx/v4/pgxpool"

	// register postgres driver
	// _ "github.com/lib/pq"
	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// NewClient creates a new postgres client
func NewClient() *sql.DB {
	dbUser := viper.GetString("POSTGRES_USER")
	dbPwd := viper.GetString("POSTGRES_PASSWORD")
	dbName := viper.GetString("POSTGRES_DB")
	dbHost := viper.GetString("POSTGRES_HOST")
	dbPort := viper.GetInt("POSTGRES_PORT")

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s", dbHost, dbPort, dbUser, dbPwd, dbName)

	db, err := sql.Open("pgx", psqlInfo)
	if err != nil {
		logrus.WithError(err).Fatal("could not open database connection")
		panic(err)
	}

	db.SetMaxOpenConns(100)

	err = db.Ping()
	if err != nil {
		panic(err)
	}
	return db
}

// NewPgxClient creates a new postgres client
func NewPgxClient() *pgxpool.Pool {
	dbUser := viper.GetString("POSTGRES_USER")
	dbPwd := viper.GetString("POSTGRES_PASSWORD")
	dbName := viper.GetString("POSTGRES_DB")
	dbHost := viper.GetString("POSTGRES_HOST")
	dbPort := viper.GetInt("POSTGRES_PORT")

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s", dbHost, dbPort, dbUser, dbPwd, dbName)

	ctx := context.Background()
	db, err := pgxpool.Connect(ctx, psqlInfo)
	if err != nil {
		logrus.WithError(err).Fatal("could not open database connection")
		panic(err)
	}

	db.Config().MaxConns = 100

	err = db.Ping(ctx)
	if err != nil {
		panic(err)
	}
	return db
}

func generateValuesPlaceholders(l, offset int) string {
	values := "("
	for i := 0; i < l; i++ {
		values += fmt.Sprintf("$%d,", i+1+offset)
	}
	return values[0:len(values)-1] + ")"
}

func checkNoErr(err error) {
	if err != nil {
		panic(err)
	}
}
