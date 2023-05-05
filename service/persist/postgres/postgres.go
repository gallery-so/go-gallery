package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/tracing"

	// register postgres driver
	_ "github.com/jackc/pgx/v4/stdlib"
	// _ "github.com/lib/pq"
)

type ErrRoleDoesNotExist struct {
	role string
}

func (e ErrRoleDoesNotExist) Error() string {
	return fmt.Sprintf("role '%s' does not exist", e.role)
}

type connectionParams struct {
	user     string
	password string
	dbname   string
	host     string
	port     int
}

func (c *connectionParams) toConnectionString() string {
	port := c.port
	if port == 0 {
		port = 5432
	}

	connStr := fmt.Sprintf("user=%s dbname=%s host=%s port=%d", c.user, c.dbname, c.host, port)

	// Empty passwords should be omitted so they don't interfere with other parameters
	// (e.g. "password= dbname=something" causes Postgres to ignore the dbname)
	if c.password != "" {
		connStr += fmt.Sprintf(" password=%s", c.password)
	}

	return connStr

	// Commented out because we should be using Cloud SQL Proxy in any context where we would have supplied
	// certificates. Keeping the code here in case we do need to allow certificates again some day.

	//countNonEmptyStrings := func(str ...string) int {
	//	numNotEmpty := 0
	//	for _, s := range str {
	//		if s != "" {
	//			numNotEmpty++
	//		}
	//	}
	//
	//	return numNotEmpty
	//}
	//
	//dbServerCa := env.GetString("POSTGRES_SERVER_CA")
	//dbClientKey := env.GetString("POSTGRES_CLIENT_KEY")
	//dbClientCert := env.GetString("POSTGRES_CLIENT_CERT")
	//
	//numSSLParams := countNonEmptyStrings(dbServerCa, dbClientKey, dbClientCert)
	//if numSSLParams == 0 {
	//	return connStr
	//} else if numSSLParams == 3 {
	//	return connStr + fmt.Sprintf(" sslmode=verify-ca sslrootcert=%s sslcert=%s sslkey=%s", dbServerCa, dbClientCert, dbClientKey)
	//}
	//
	//panic(fmt.Errorf("POSTGRES_SERVER_CA, POSTGRES_CLIENT_KEY, and POSTGRES_CLIENT_CERT must be set together (all must have values or all must be empty)"))
}

func newConnectionParamsFromEnv() connectionParams {
	return connectionParams{
		user:     env.GetString("POSTGRES_USER"),
		password: env.GetString("POSTGRES_PASSWORD"),
		dbname:   env.GetString("POSTGRES_DB"),
		host:     env.GetString("POSTGRES_HOST"),
		port:     env.GetInt("POSTGRES_PORT"),
	}
}

type ConnectionOption func(params *connectionParams)

func WithUser(user string) ConnectionOption {
	return func(params *connectionParams) {
		params.user = user
	}
}

func WithPassword(password string) ConnectionOption {
	return func(params *connectionParams) {
		params.password = password
	}
}

func WithDBName(dbname string) ConnectionOption {
	return func(params *connectionParams) {
		params.dbname = dbname
	}
}

func WithHost(host string) ConnectionOption {
	return func(params *connectionParams) {
		params.host = host
	}
}

func WithPort(port int) ConnectionOption {
	return func(params *connectionParams) {
		params.port = port
	}
}

// MustCreateClient panics when it fails to create a new database connection
func MustCreateClient(opts ...ConnectionOption) *sql.DB {
	db, err := NewClient(opts...)
	if err != nil {
		panic(err)
	}
	return db
}

func NewClient(opts ...ConnectionOption) (*sql.DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	params := newConnectionParamsFromEnv()
	for _, opt := range opts {
		opt(&params)
	}

	db, err := sql.Open("pgx", params.toConnectionString())
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(50)

	err = db.PingContext(ctx)
	if err != nil && strings.Contains(err.Error(), fmt.Sprintf("role \"%s\" does not exist", params.user)) {
		return nil, ErrRoleDoesNotExist{params.user}
	}
	if err != nil {
		return nil, err
	}
	return db, nil
}

// NewPgxClient creates a new postgres client via pgx
func NewPgxClient(opts ...ConnectionOption) *pgxpool.Pool {
	ctx := context.Background()

	params := newConnectionParamsFromEnv()
	for _, opt := range opts {
		opt(&params)
	}

	config, err := pgxpool.ParseConfig(params.toConnectionString())
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

// Repositories is the set of all available persistence repositories
type Repositories struct {
	db                    *sql.DB
	pool                  *pgxpool.Pool
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

func NewRepositories(pq *sql.DB, pgx *pgxpool.Pool) *Repositories {
	queries := coredb.New(pgx)

	return &Repositories{
		db:                    pq,
		pool:                  pgx,
		UserRepository:        NewUserRepository(pq, queries, pgx),
		NonceRepository:       NewNonceRepository(pq, queries),
		TokenRepository:       NewTokenGalleryRepository(pq, queries),
		CollectionRepository:  NewCollectionTokenRepository(pq, queries),
		GalleryRepository:     NewGalleryRepository(queries),
		ContractRepository:    NewContractGalleryRepository(pq, queries),
		MembershipRepository:  NewMembershipRepository(pq, queries),
		EarlyAccessRepository: NewEarlyAccessRepository(pq, queries),
		WalletRepository:      NewWalletRepository(pq, queries),
		AdmireRepository:      NewAdmireRepository(queries),
		CommentRepository:     NewCommentRepository(pq, queries),
	}
}

func (r *Repositories) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.BeginTx(ctx, pgx.TxOptions{})
}
