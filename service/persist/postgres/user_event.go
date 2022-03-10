package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

type UserEventRepository struct {
	db                           *sql.DB
	createStmt                   *sql.Stmt
	getByEventIDStmt             *sql.Stmt
	getMatchingEventsForUserStmt *sql.Stmt
}

func NewUserEventRepository(db *sql.DB) *UserEventRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx,
		`INSERT INTO user_events (ID, USER_ID, VERSION, EVENT_CODE, DATA) VALUES ($1, $2, $3, $4, $5)
		 RETURNING ID;`,
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

	return &UserEventRepository{
		db:                           db,
		createStmt:                   createStmt,
		getByEventIDStmt:             getByEventIDStmt,
		getMatchingEventsForUserStmt: getMatchingEventsForUserStmt,
	}
}

func (e *UserEventRepository) Add(ctx context.Context, event persist.UserEventRecord) (persist.DBID, error) {
	var id persist.DBID
	err := e.createStmt.QueryRowContext(ctx, persist.GenerateID(), event.UserID, event.Version, event.Code, event.Data).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
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
		var event persist.UserEventRecord
		if err := res.Scan(&event.ID, &event.UserID, &event.Version, &event.Code, &event.Data, &event.CreationTime, &event.LastUpdated); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}
