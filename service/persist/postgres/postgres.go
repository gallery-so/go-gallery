package postgres

import (
	"database/sql"
	"fmt"
	"reflect"

	// register postgres driver
	_ "github.com/lib/pq"
	"github.com/spf13/viper"
)

// NewPostgresClient creates a new postgres client
func NewPostgresClient() *sql.DB {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		viper.GetString("POSTGRES_HOST"), viper.GetInt("POSTGRES_PORT"), viper.GetString("POSTGRES_USER"), viper.GetString("POSTGRES_PASSWORD"), viper.GetString("POSTGRES_DB"))

	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		panic(err)
	}

	err = db.Ping()
	if err != nil {
		panic(err)
	}
	return db
}

func prepareSet(update interface{}) string {
	val := reflect.ValueOf(update)
	tp := reflect.TypeOf(update)

	set := "SET "
	for i := 0; i < val.NumField(); i++ {
		name, ok := tp.Field(i).Tag.Lookup("postgres")
		if !ok {
			continue
		}
		set += fmt.Sprintf("%s = $%d,", name, i+1)
	}

	return set[0 : len(set)-1]

}

func generateValuesPlaceholders(l, offset int) string {
	values := "("
	for i := 0; i < l; i++ {
		values += fmt.Sprintf("$%d,", i+1+offset)
	}
	return values[0:len(values)-1] + ")"
}
