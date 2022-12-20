package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/mikeydub/go-gallery/service/persist"
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
	if dbPort == 0 {
		dbPort = 5432
	}

	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s", dbHost, dbPort, dbUser, dbPwd, dbName)

	dbServerCa := viper.GetString("POSTGRES_SERVER_CA")
	dbClientKey := viper.GetString("POSTGRES_CLIENT_KEY")
	dbClientCert := viper.GetString("POSTGRES_CLIENT_CERT")

	numSSLParams := countNonEmptyStrings(dbServerCa, dbClientKey, dbClientCert)
	if numSSLParams == 0 {
		return connStr
	} else if numSSLParams == 3 {
		return connStr + fmt.Sprintf(" sslmode=verify-ca sslrootcert=%s sslcert=%s sslkey=%s", dbServerCa, dbClientCert, dbClientKey)
	}

	panic(fmt.Errorf("POSTGRES_SERVER_CA, POSTGRES_CLIENT_KEY, and POSTGRES_CLIENT_CERT must be set together (all must have values or all must be empty)"))
}

func countNonEmptyStrings(str ...string) int {
	numNotEmpty := 0
	for _, s := range str {
		if s != "" {
			numNotEmpty++
		}
	}

	return numNotEmpty
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

	// Get the current time before we do anything else, since this is our best approximation
	// of when the operation "finished"
	endTime := time.Now()

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

	// Current disabled; re-enable if/when we have handling for sensitive information.
	// Otherwise, we'll send every SQL parameter to Sentry, which could contain PII.
	//if args, ok := data["args"]; ok {
	//	spanData["sql args"] = args
	//}

	tracing.AddEventDataToSpan(span, spanData)

	// pgx calls the logger AFTER the operation happens, but it tells us how long the operation took.
	// We can use that to update our span so it reflects the correct start time.
	span.EndTime = endTime
	span.StartTime = endTime.Add(-duration)
}

func generateValuesPlaceholders(l, offset int, nows []int) string {
	indexToNow := make(map[int]bool)
	if nows != nil {
		for _, i := range nows {
			indexToNow[i] = true
		}
	}
	values := "("
	d := 0
	for i := 0; i < l; i++ {
		if indexToNow[i] {
			values += "now()"
		} else {
			values += fmt.Sprintf("$%d,", d+1+offset)
			d++
		}
	}
	return values[0:len(values)-1] + ")"
}

func checkNoErr(err error) {
	if err != nil {
		panic(err)
	}
}

func dbidsToStrings(dbids []persist.DBID) []string {
	strings := make([]string, len(dbids))
	for i, dbid := range dbids {
		strings[i] = string(dbid)
	}
	return strings
}

// Repositories is the set of all available persistence repositories
type Repositories struct {
	UserRepository        *UserRepository
	NonceRepository       *NonceRepository
	GalleryRepository     *GalleryRepository
	TokenRepository       *TokenGalleryRepository
	CollectionRepository  *CollectionTokenRepository
	ContractRepository    *ContractGalleryRepository
	MembershipRepository  *MembershipRepository
	EarlyAccessRepository *EarlyAccessRepository
	WalletRepository      *WalletRepository
	AdmireRepository      *AdmireRepository
	CommentRepository     *CommentRepository
}
