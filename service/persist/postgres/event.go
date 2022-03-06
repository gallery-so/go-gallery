package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

type EventRepository struct {
	db                           *sql.DB
	createStmt                   *sql.Stmt
	getByEventIDStmt             *sql.Stmt
	getMatchingEventsForUserStmt *sql.Stmt
}

func NewEventRepository(db *sql.DB) *EventRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx,
		`INSERT INTO events (ID, USER_ID, VERSION, TYPE, MESSAGE) ($1, $2, $3, $4, $5)
		 RETURNING ID;`,
	)
	checkNoErr(err)

	getByEventIDStmt, err := db.PrepareContext(ctx,
		`SELECT ID, USER_ID, VERSION, TYPE, CREATED_AT, LAST_UPDATED
		 FROM events WHERE ID = $1;`,
	)
	checkNoErr(err)

	getMatchingEventsForUserStmt, err := db.PrepareContext(ctx,
		`SELECT ID, USER_ID, VERSION, TYPE, CREATED_AT, LAST_UPDATED
		 FROM events
		 WHERE USER_ID = $1 AND EVENT_TYPE = $2 AND CREATED_AT > $3 AND CREATED_AT <= $4;`,
	)
	checkNoErr(err)

	return &EventRepository{
		db:                           db,
		createStmt:                   createStmt,
		getByEventIDStmt:             getByEventIDStmt,
		getMatchingEventsForUserStmt: getMatchingEventsForUserStmt,
	}
}

func (e *EventRepository) Create(ctx context.Context, event persist.Event) (persist.DBID, error) {
	var id persist.DBID
	err := e.createStmt.QueryRowContext(ctx, persist.GenerateID(), event.UserID, event.Version, event.Type, event.Message).Scan(&id)
	if err != nil {
		return "", nil
	}
	return id, nil
}

func (e *EventRepository) Get(ctx context.Context, eventID persist.DBID) (persist.Event, error) {
	var event persist.Event
	err := e.getByEventIDStmt.QueryRowContext(ctx, eventID).Scan(&event)
	if err != nil {
		return persist.Event{}, err
	}
	return event, nil
}

func (e *EventRepository) GetMatchingEventsForUser(ctx context.Context, event persist.Event, since time.Time) ([]persist.Event, error) {
	res, err := e.getMatchingEventsForUserStmt.QueryContext(ctx, event.UserID, event.Type, event.CreationTime, since)
	if err != nil {
		return []persist.Event{}, err
	}

	events := make([]persist.Event, 0)
	for res.Next() {
		var event persist.Event
		if err := res.Scan(&event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}
