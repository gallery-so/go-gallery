package postgres

import (
	"database/sql"
	"fmt"

	// register postgres driver
	_ "github.com/lib/pq"
	"github.com/spf13/viper"
)

// NewClient creates a new postgres client
func NewClient() *sql.DB {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		viper.GetString("POSTGRES_HOST"), viper.GetInt("POSTGRES_PORT"), viper.GetString("POSTGRES_USER"), viper.GetString("POSTGRES_PASSWORD"), viper.GetString("POSTGRES_DB"))

	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		panic(err)
	}

	err = db.Ping()
	if err != nil {
		panic(fmt.Sprintf("Error connecting to postgres: %s %T", err, err))
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
