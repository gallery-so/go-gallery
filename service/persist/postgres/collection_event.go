package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

type CollectionEventRepository struct {
	db                                       *sql.DB
	createStmt                               *sql.Stmt
	getByEventIDStmt                         *sql.Stmt
	getMatchingEventForUserAndCollectionStmt *sql.Stmt
	getMatchingEventBeforeStmt               *sql.Stmt
}

func NewCollectionEventRepository(db *sql.DB) *CollectionEventRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx,
		`INSERT INTO collection_events (ID, USER_ID, COLLECTION_ID, VERSION, EVENT_CODE, DATA) VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING ID, USER_ID, COLLECTION_ID, VERSION, EVENT_CODE, DATA, CREATED_AT, LAST_UPDATED;`,
	)
	checkNoErr(err)

	getByEventIDStmt, err := db.PrepareContext(ctx,
		`SELECT ID, USER_ID, COLLECTION_ID, VERSION, EVENT_CODE, DATA, CREATED_AT, LAST_UPDATED
		 FROM collection_events WHERE ID = $1;`,
	)
	checkNoErr(err)

	getMatchingEventForUserAndCollectionStmt, err := db.PrepareContext(ctx,
		`SELECT ID, USER_ID, COLLECTION_ID, VERSION, EVENT_CODE, DATA, CREATED_AT, LAST_UPDATED
		 FROM collection_events
		 WHERE USER_ID = $1 AND COLLECTION_ID = $2 AND EVENT_CODE = $3 AND CREATED_AT > $4 AND CREATED_AT <= $5;`,
	)
	checkNoErr(err)

	getMatchingEventBeforeStmt, err := db.PrepareContext(ctx,
		`SELECT ID, USER_ID, COLLECTION_ID, VERSION, EVENT_CODE, DATA, CREATED_AT, LAST_UPDATED
		 FROM collection_events
		 WHERE USER_ID = $1 AND COLLECTION_ID = $2 AND EVENT_CODE = $3 AND LAST_UPDATED < $4
		 ORDER BY LAST_UPDATED DESC LIMIT 1`,
	)
	checkNoErr(err)

	return &CollectionEventRepository{
		db:                                       db,
		createStmt:                               createStmt,
		getByEventIDStmt:                         getByEventIDStmt,
		getMatchingEventForUserAndCollectionStmt: getMatchingEventForUserAndCollectionStmt,
		getMatchingEventBeforeStmt:               getMatchingEventBeforeStmt,
	}
}

func (e *CollectionEventRepository) Add(ctx context.Context, event persist.CollectionEventRecord) (*persist.CollectionEventRecord, error) {
	var evt persist.CollectionEventRecord
	err := e.createStmt.QueryRowContext(ctx, persist.GenerateID(), event.UserID, event.CollectionID, event.Version, event.Code, event.Data).Scan(
		&evt.ID, &evt.UserID, &evt.CollectionID, &evt.Version, &evt.Code, &evt.Data, &evt.CreationTime, &evt.LastUpdated)
	if err != nil {
		return nil, err
	}
	return &evt, nil
}

func (e *CollectionEventRepository) Get(ctx context.Context, eventID persist.DBID) (persist.CollectionEventRecord, error) {
	var event persist.CollectionEventRecord
	err := e.getByEventIDStmt.QueryRowContext(ctx, eventID).Scan(&event.ID, &event.UserID, &event.CollectionID, &event.Version, &event.Code, &event.Data, &event.CreationTime, &event.LastUpdated)
	if err != nil {
		return persist.CollectionEventRecord{}, nil
	}
	return event, nil
}

func (e *CollectionEventRepository) GetEventsSince(ctx context.Context, event persist.CollectionEventRecord, since time.Time) ([]persist.CollectionEventRecord, error) {
	res, err := e.getMatchingEventForUserAndCollectionStmt.QueryContext(ctx, event.UserID, event.CollectionID, event.Code, event.CreationTime, since)
	if err != nil {
		return nil, err
	}
	events := make([]persist.CollectionEventRecord, 0)
	for res.Next() {
		var evt persist.CollectionEventRecord
		if err := res.Scan(&evt.ID, &evt.UserID, &evt.CollectionID, &evt.Version, &evt.Code, &evt.Data, &evt.CreationTime, &evt.LastUpdated); err != nil {
			return nil, err
		}
		events = append(events, evt)
	}
	return events, nil
}

func (e *CollectionEventRepository) GetEventBefore(ctx context.Context, event persist.CollectionEventRecord) (*persist.CollectionEventRecord, error) {
	var evt persist.CollectionEventRecord
	err := e.getMatchingEventBeforeStmt.QueryRowContext(ctx, event.UserID, event.CollectionID, event.Code, event.CreationTime).Scan(&evt.ID, &evt.UserID, &evt.CollectionID, &evt.Version, &evt.Code, &evt.Data, &evt.CreationTime, &evt.LastUpdated)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &evt, nil
}
