package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

type CollectionEventRepository struct {
	db                                       *sql.DB
	createStmt                               *sql.Stmt
	getByEventIDStmt                         *sql.Stmt
	getMatchingEventForUserAndCollectionStmt *sql.Stmt
}

func NewCollectionEventRepository(db *sql.DB) *CollectionEventRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx,
		`INSERT INTO collection_events (ID, USER_ID, COLLECTION_ID, VERSION, EVENT_TYPE, EVENT) VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING ID;`,
	)
	checkNoErr(err)

	getByEventIDStmt, err := db.PrepareContext(ctx,
		`SELECT ID, USER_ID, COLLECTION_ID, VERSION, EVENT_TYPE, EVENT, CREATED_AT, LAST_UPDATED
		 FROM collection_events WHERE ID = $1;`,
	)
	checkNoErr(err)

	getMatchingEventForUserAndCollectionStmt, err := db.PrepareContext(ctx,
		`SELECT ID, USER_ID, COLLECTION_ID, VERSION, EVENT_TYPE, EVENT, CREATED_AT, LAST_UPDATED
		 FROM collection_events
		 WHERE USER_ID = $1 AND COLLECTION_ID = $2 AND EVENT_TYPE = $3 AND CREATED_AT > $4 AND CREATED_AT <= $5;`,
	)
	checkNoErr(err)

	return &CollectionEventRepository{
		db:                                       db,
		createStmt:                               createStmt,
		getByEventIDStmt:                         getByEventIDStmt,
		getMatchingEventForUserAndCollectionStmt: getMatchingEventForUserAndCollectionStmt,
	}
}

type errFailedToFetchCollectionEvent struct {
	eventID persist.DBID
}

func (e errFailedToFetchCollectionEvent) Retryable() bool {
	return true
}

func (e errFailedToFetchCollectionEvent) Error() string {
	return fmt.Sprintf("event does not exist: %s", e.eventID)
}

func (e *CollectionEventRepository) Add(ctx context.Context, event persist.CollectionEventRecord) (persist.DBID, error) {
	var id persist.DBID
	err := e.createStmt.QueryRowContext(ctx, persist.GenerateID(), event.UserID, event.CollectionID, event.Version, event.Type, event.Event).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (e *CollectionEventRepository) Get(ctx context.Context, eventID persist.DBID) (persist.CollectionEventRecord, error) {
	var event persist.CollectionEventRecord
	err := e.getByEventIDStmt.QueryRowContext(ctx, eventID).Scan(&event)
	if err == sql.ErrNoRows {
		return persist.CollectionEventRecord{}, err
	}
	if err != nil {
		return persist.CollectionEventRecord{}, errFailedToFetchCollectionEvent{event.ID}
	}
	return event, nil
}

func (e *CollectionEventRepository) GetEventsSince(ctx context.Context, event persist.CollectionEventRecord, since time.Time) ([]persist.CollectionEventRecord, error) {
	res, err := e.getMatchingEventForUserAndCollectionStmt.QueryContext(ctx, event.UserID, event.CollectionID, event.Type, event.CreationTime, since)
	if err != nil {
		return []persist.CollectionEventRecord{}, errFailedToFetchCollectionEvent{event.ID}
	}
	events := make([]persist.CollectionEventRecord, 0)
	for res.Next() {
		var event persist.CollectionEventRecord
		if err := res.Scan(&event); err != nil {
			return nil, errFailedToFetchCollectionEvent{event.ID}
		}
		events = append(events, event)
	}
	return events, nil
}
