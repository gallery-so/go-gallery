package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

type TokenEventRepository struct {
	db                                   *sql.DB
	createStmt                           *sql.Stmt
	getByEventIDStmt                     *sql.Stmt
	getMatchingEventsForUserAndTokenStmt *sql.Stmt
}

func NewTokenEventRepository(db *sql.DB) *TokenEventRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx,
		`INSERT INTO token_events (ID, USER_ID, TOKEN_ID, VERSION, EVENT_CODE, DATA) VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING ID;`,
	)
	checkNoErr(err)

	getByEventIDStmt, err := db.PrepareContext(ctx,
		`SELECT ID, USER_ID, TOKEN_ID, VERSION, EVENT_CODE, DATA, CREATED_AT, LAST_UPDATED
		 FROM token_events WHERE ID = $1;`,
	)
	checkNoErr(err)

	getMatchingEventsForUserAndTokenStmt, err := db.PrepareContext(ctx,
		`SELECT ID, USER_ID, TOKEN_ID, VERSION, EVENT_CODE, DATA, CREATED_AT, LAST_UPDATED
		 FROM token_events
		 WHERE USER_ID = $1 AND TOKEN_ID = $2 AND EVENT_CODE = $3 AND CREATED_AT > $4 AND CREATED_AT <= $5;`,
	)
	checkNoErr(err)

	return &TokenEventRepository{
		db:                                   db,
		createStmt:                           createStmt,
		getByEventIDStmt:                     getByEventIDStmt,
		getMatchingEventsForUserAndTokenStmt: getMatchingEventsForUserAndTokenStmt,
	}
}

type errFailedToFetchTokenEvent struct {
	eventID persist.DBID
}

func (e errFailedToFetchTokenEvent) Retryable() bool {
	return true
}

func (e errFailedToFetchTokenEvent) Error() string {
	return fmt.Sprintf("event does not exist: %s", e.eventID)
}

func (e *TokenEventRepository) Add(ctx context.Context, event persist.TokenEventRecord) (persist.DBID, error) {
	var id persist.DBID
	err := e.createStmt.QueryRowContext(ctx, persist.GenerateID(), event.UserID, event.TokenID, event.Version, event.Code, event.Data).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (e *TokenEventRepository) Get(ctx context.Context, eventID persist.DBID) (persist.TokenEventRecord, error) {
	var event persist.TokenEventRecord
	err := e.getByEventIDStmt.QueryRowContext(ctx, eventID).Scan(&event)
	if err == sql.ErrNoRows {
		return persist.TokenEventRecord{}, err
	}
	if err != nil {
		return persist.TokenEventRecord{}, errFailedToFetchTokenEvent{event.ID}
	}
	return event, nil
}

func (e *TokenEventRepository) GetEventsSince(ctx context.Context, event persist.TokenEventRecord, since time.Time) ([]persist.TokenEventRecord, error) {
	res, err := e.getMatchingEventsForUserAndTokenStmt.QueryContext(ctx, event.UserID, event.TokenID, event.Code, event.CreationTime, since)
	if err != nil {
		return []persist.TokenEventRecord{}, errFailedToFetchTokenEvent{event.ID}
	}
	events := make([]persist.TokenEventRecord, 0)
	for res.Next() {
		var event persist.TokenEventRecord
		if err := res.Scan(&event); err != nil {
			return nil, errFailedToFetchTokenEvent{event.ID}
		}
		events = append(events, event)
	}
	return events, nil
}
