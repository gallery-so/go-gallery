package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

type NftEventRepository struct {
	db                                 *sql.DB
	createStmt                         *sql.Stmt
	getByEventIDStmt                   *sql.Stmt
	getMatchingEventsForUserAndNftStmt *sql.Stmt
}

func NewNftEventRepository(db *sql.DB) *NftEventRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx,
		`INSERT INTO nft_events (ID, USER_ID, NFT_ID, VERSION, EVENT_CODE, DATA) VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING ID;`,
	)
	checkNoErr(err)

	getByEventIDStmt, err := db.PrepareContext(ctx,
		`SELECT ID, USER_ID, NFT_ID, VERSION, EVENT_CODE, DATA, CREATED_AT, LAST_UPDATED
		 FROM nft_events WHERE ID = $1;`,
	)
	checkNoErr(err)

	getMatchingEventsForUserAndNftStmt, err := db.PrepareContext(ctx,
		`SELECT ID, USER_ID, NFT_ID, VERSION, EVENT_CODE, DATA, CREATED_AT, LAST_UPDATED
		 FROM nft_events
		 WHERE USER_ID = $1 AND NFT_ID = $2 AND EVENT_CODE = $3 AND CREATED_AT > $4 AND CREATED_AT <= $5;`,
	)
	checkNoErr(err)

	return &NftEventRepository{
		db:                                 db,
		createStmt:                         createStmt,
		getByEventIDStmt:                   getByEventIDStmt,
		getMatchingEventsForUserAndNftStmt: getMatchingEventsForUserAndNftStmt,
	}
}

func (e *NftEventRepository) Add(ctx context.Context, event persist.NftEventRecord) (persist.DBID, error) {
	var id persist.DBID
	err := e.createStmt.QueryRowContext(ctx, persist.GenerateID(), event.UserID, event.NftID, event.Version, event.Code, event.Data).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (e *NftEventRepository) Get(ctx context.Context, eventID persist.DBID) (persist.NftEventRecord, error) {
	var event persist.NftEventRecord
	err := e.getByEventIDStmt.QueryRowContext(ctx, eventID).Scan(&event.ID, &event.UserID, &event.NftID,
		&event.Version, &event.Code, &event.Data, &event.CreationTime, &event.NftID)
	if err != nil {
		return persist.NftEventRecord{}, err
	}
	return event, nil
}

func (e *NftEventRepository) GetEventsSince(ctx context.Context, event persist.NftEventRecord, since time.Time) ([]persist.NftEventRecord, error) {
	res, err := e.getMatchingEventsForUserAndNftStmt.QueryContext(ctx, event.UserID, event.NftID, event.Code, event.CreationTime, since)
	if err != nil {
		return nil, err
	}
	events := make([]persist.NftEventRecord, 0)
	for res.Next() {
		var event persist.NftEventRecord
		if err := res.Scan(&event.ID, &event.UserID, &event.NftID, &event.Version, &event.Code, &event.Data, &event.CreationTime, &event.NftID); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}
