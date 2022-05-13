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

func getSqlConnectionString() string {
	dbUser := viper.GetString("POSTGRES_USER")
	dbPwd := viper.GetString("POSTGRES_PASSWORD")
	dbName := viper.GetString("POSTGRES_DB")
	dbHost := viper.GetString("POSTGRES_HOST")
	dbPort := viper.GetInt("POSTGRES_PORT")

	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s", dbHost, dbPort, dbUser, dbPwd, dbName)
}

// NewClient creates a new postgres client
func NewClient() *sql.DB {
	db, err := sql.Open("pgx", getSqlConnectionString())
	if err != nil {
		logrus.WithError(err).Fatal("could not open database connection")
		panic(err)
	}

	db.SetMaxOpenConns(50)

	err = db.Ping()
	if err != nil {
		panic(err)
	}
	return db
}

// NewPgxClient creates a new postgres client via pgx
func NewPgxClient() *pgxpool.Pool {
	ctx := context.Background()
	db, err := pgxpool.Connect(ctx, getSqlConnectionString())
	if err != nil {
		logrus.WithError(err).Fatal("could not open database connection")
		panic(err)
	}

	// Split 50/50 with existing database/sql implementation so we don't go over the GCP limit
	// for incoming connections. Once we remove database/sql, this can go back up to 100.
	db.Config().MaxConns = 50

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
