package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/tracing"

	// register postgres driver
	_ "github.com/jackc/pgx/v4/stdlib"
	// _ "github.com/lib/pq"
	"github.com/spf13/viper"
)

func getSqlConnectionString() string {
	dbUser := viper.GetString("POSTGRES_USER")
	dbPwd := viper.GetString("POSTGRES_PASSWORD")
	dbName := viper.GetString("POSTGRES_DB")
	dbHost := viper.GetString("POSTGRES_HOST")
	dbPort := viper.GetInt("POSTGRES_PORT")

	if viper.GetString("ENV") != "local" {
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s", dbHost, dbPort, dbUser, dbPwd, dbName)
	} else {
		dbServerCa := viper.GetString("POSTGRES_SERVER_CA")
		dbClientKey := viper.GetString("POSTGRES_CLIENT_KEY")
		dbClientCert := viper.GetString("POSTGRES_CLIENT_CERT")

		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=verify-ca sslrootcert=%s sslcert=%s sslkey=%s", dbHost, dbPort, dbUser, dbPwd, dbName, dbServerCa, dbClientCert, dbClientKey)
	}

}

// NewClient creates a new postgres client
func NewClient() *sql.DB {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	db, err := sql.Open("pgx", getSqlConnectionString())
	if err != nil {
		logger.For(nil).WithError(err).Fatal("could not open database connection")
		panic(err)
	}

	db.SetMaxOpenConns(50)

	err = db.PingContext(ctx)
	if err != nil {
		panic(err)
	}
	return db
}

// NewPgxClient creates a new postgres client via pgx
func NewPgxClient() *pgxpool.Pool {
	ctx := context.Background()

	config, err := pgxpool.ParseConfig(getSqlConnectionString())
	if err != nil {
		logger.For(nil).WithError(err).Fatal("could not parse pgx connection string")
		panic(err)
	}

	config.ConnConfig.Logger = &pgxTracer{continueOnly: true}

	db, err := pgxpool.ConnectConfig(ctx, config)
	if err != nil {
		logger.For(nil).WithError(err).Fatal("could not open database connection")
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

type pgxTracer struct {
	continueOnly bool
}

func (l *pgxTracer) Log(ctx context.Context, level pgx.LogLevel, msg string, data map[string]interface{}) {
	if data == nil {
		return
	}

	if l.continueOnly {
		transaction := sentry.TransactionFromContext(ctx)
		if transaction == nil {
			return
		}
	}

	// Only trace things that have a duration
	duration, ok := data["time"].(time.Duration)
	if !ok {
		return
	}

	operation := "other"
	if strings.EqualFold(msg, "query") {
		operation = "query"
	} else if strings.EqualFold(msg, "exec") {
		operation = "exec"
	}

	description := msg

	sqlStr, ok := data["sql"].(string)
	if ok {
		// If a SQL statement was supplied, use that as the default description
		description = sqlStr

		// If it's a sqlc query, try to parse the query name for an even better description
		const sqlcPrefix = "-- name: "
		if strings.HasPrefix(sqlStr, sqlcPrefix) && len(sqlStr) > len(sqlcPrefix) {
			withoutPrefix := sqlStr[len(sqlcPrefix):]
			if spaceIndex := strings.Index(withoutPrefix, " "); spaceIndex != -1 {
				description = withoutPrefix[:spaceIndex]
			}
		}
	}

	span, ctx := tracing.StartSpan(ctx, "db."+operation, description)
	defer tracing.FinishSpan(span)

	spanData := map[string]interface{}{
		"logMessage": msg,
	}

	if sqlStr != "" {
		spanData["sql"] = sqlStr
	}

	if rows, ok := data["rowCount"]; ok {
		spanData["rowCount"] = rows
	}

	if args, ok := data["args"]; ok {
		spanData["sql args"] = args
	}

	tracing.AddEventDataToSpan(span, spanData)

	// pgx calls the logger AFTER the operation happens, but it tells us how long the operation took.
	// We can use that to update our span so it reflects the correct start time.
	span.StartTime = time.Now().Add(-duration)
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
