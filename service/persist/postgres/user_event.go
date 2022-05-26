package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

type UserEventRepository struct {
	db                           *sql.DB
	createStmt                   *sql.Stmt
	getByEventIDStmt             *sql.Stmt
	getMatchingEventsForUserStmt *sql.Stmt
	getMatchingEventBeforeStmt   *sql.Stmt
	markSentStmt                 *sql.Stmt
}

func NewUserEventRepository(db *sql.DB) *UserEventRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx,
		`INSERT INTO user_events (ID, USER_ID, VERSION, EVENT_CODE, DATA) VALUES ($1, $2, $3, $4, $5)
		 RETURNING ID, USER_ID, VERSION, EVENT_CODE, DATA, CREATED_AT, LAST_UPDATED;`,
	)
	checkNoErr(err)

	getByEventIDStmt, err := db.PrepareContext(ctx,
		`SELECT ID, USER_ID, VERSION, EVENT_CODE, DATA, CREATED_AT, LAST_UPDATED
		 FROM user_events WHERE ID = $1;`,
	)
	checkNoErr(err)

	getMatchingEventsForUserStmt, err := db.PrepareContext(ctx,
		`SELECT ID, USER_ID, VERSION, EVENT_CODE, DATA, CREATED_AT, LAST_UPDATED
		 FROM user_events
		 WHERE USER_ID = $1 AND EVENT_CODE = $2 AND CREATED_AT > $3 AND CREATED_AT <= $4;`,
	)
	checkNoErr(err)

	getMatchingEventBeforeStmt, err := db.PrepareContext(ctx,
		`SELECT ID, USER_ID, VERSION, EVENT_CODE, DATA, CREATED_AT, LAST_UPDATED
		 FROM user_events
		 WHERE USER_ID = $1 AND EVENT_CODE = $2 AND LAST_UPDATED < $3 AND SENT = true
		 ORDER BY LAST_UPDATED DESC LIMIT 1`,
	)
	checkNoErr(err)

	markSentStmt, err := db.PrepareContext(ctx, `UPDATE user_events SET SENT = true, LAST_UPDATED = now() WHERE ID = $1`)
	checkNoErr(err)

	return &UserEventRepository{
		db:                           db,
		createStmt:                   createStmt,
		getByEventIDStmt:             getByEventIDStmt,
		getMatchingEventsForUserStmt: getMatchingEventsForUserStmt,
		getMatchingEventBeforeStmt:   getMatchingEventBeforeStmt,
		markSentStmt:                 markSentStmt,
	}
}

func (e *UserEventRepository) Add(ctx context.Context, event persist.UserEventRecord) (*persist.UserEventRecord, error) {
	var evt persist.UserEventRecord
	err := e.createStmt.QueryRowContext(ctx, persist.GenerateID(), event.UserID, event.Version, event.Code, event.Data).Scan(
		&evt.ID, &evt.UserID, &evt.Version, &evt.Code, &evt.Data, &evt.CreationTime, &evt.LastUpdated)
	if err != nil {
		return nil, err
	}
	return &evt, nil
}

func (e *UserEventRepository) Get(ctx context.Context, eventID persist.DBID) (persist.UserEventRecord, error) {
	var event persist.UserEventRecord
	err := e.getByEventIDStmt.QueryRowContext(ctx, eventID).Scan(&event.ID, &event.UserID, &event.Version, &event.Code, &event.Data, &event.CreationTime, &event.LastUpdated)
	if err != nil {
		return persist.UserEventRecord{}, err
	}
	return event, nil
}

func (e *UserEventRepository) GetEventsSince(ctx context.Context, event persist.UserEventRecord, since time.Time) ([]persist.UserEventRecord, error) {
	res, err := e.getMatchingEventsForUserStmt.QueryContext(ctx, event.UserID, event.Code, event.CreationTime, since)
	if err != nil {
		return nil, err
	}
	events := make([]persist.UserEventRecord, 0)
	for res.Next() {
		var evt persist.UserEventRecord
		if err := res.Scan(&evt.ID, &evt.UserID, &evt.Version, &evt.Code, &evt.Data, &evt.CreationTime, &evt.LastUpdated); err != nil {
			return nil, err
		}
		events = append(events, evt)
	}
	return events, nil
}

func (e *UserEventRepository) GetEventBefore(ctx context.Context, event persist.UserEventRecord) (*persist.UserEventRecord, error) {
	var evt persist.UserEventRecord
	err := e.getMatchingEventBeforeStmt.QueryRowContext(ctx, event.UserID, event.Code, event.CreationTime).Scan(&evt.ID, &evt.UserID, &evt.Version, &evt.Code, &evt.Data, &evt.CreationTime, &evt.LastUpdated)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &evt, nil
}

func (e *UserEventRepository) MarkSent(ctx context.Context, eventID persist.DBID) error {
	res, err := e.markSentStmt.ExecContext(ctx, eventID)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return fmt.Errorf("user event(%s) doesn't exist", eventID)
	}

	return nil
}
