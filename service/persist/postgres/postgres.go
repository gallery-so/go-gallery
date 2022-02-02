package postgres

import (
	"database/sql"
	"fmt"

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

	var psqlInfo string
	if viper.GetString("ENV") != "local" {
		psqlInfo = fmt.Sprintf("user=%s password=%s dbname=%s host=%s", dbUser, dbPwd, dbName, dbHost)
	} else {
		psqlInfo = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", dbHost, dbPort, dbUser, dbPwd, dbName)
	}

	db, err := sql.Open("pgx", psqlInfo)
	if err != nil {
		logrus.WithError(err).Fatal("could not open database connection")
		panic(err)
	}

	err = db.Ping()
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
