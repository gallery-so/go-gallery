package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

// AccessRepository represents an access repository in the postgres database
type AccessRepository struct {
	db                               *sql.DB
	getByUserIDStmt                  *sql.Stmt
	hasRequiredTokensStmt            *sql.Stmt
	upsertRequiredTokensByUserIDStmt *sql.Stmt
}

// NewAccessRepository creates a new postgres repository for interacting with accesses
func NewAccessRepository(db *sql.DB) *AccessRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	getByUserIDStmt, err := db.PrepareContext(ctx, `SELECT ID,CREATED_AT,LAST_UPDATED,DELETED,VERSION,USER_ID,MOST_RECENT_BLOCK,REQUIRED_TOKENS_OWNED,IS_ADMIN FROM access WHERE USER_ID = $1 LIMIT 1`)
	checkNoErr(err)

	hasRequiredTokensStmt, err := db.PrepareContext(ctx, `SELECT ID FROM access WHERE USER_ID = $1 AND MOST_RECENT_BLOCK = (SELECT MAX(MOST_RECENT_BLOCK) FROM access WHERE USER_ID = $1) AND REQUIRED_TOKENS -> ALL($2) IS NOT NULL`)
	checkNoErr(err)

	upsertRequiredTokensByUserIDStmt, err := db.PrepareContext(ctx, `INSERT INTO access (ID,USER_ID,MOST_RECENT_BLOCK,REQUIRED_TOKENS,IS_ADMIN) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (USER_ID) DO UPDATE SET REQUIRED_TOKENS = $4, MOST_RECENT_BLOCK = $3`)
	checkNoErr(err)

	return &AccessRepository{db: db, getByUserIDStmt: getByUserIDStmt, hasRequiredTokensStmt: hasRequiredTokensStmt, upsertRequiredTokensByUserIDStmt: upsertRequiredTokensByUserIDStmt}
}

// GetByUserID returns the access for the given user ID
func (a *AccessRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID) (persist.Access, error) {

	access := persist.Access{}
	err := a.getByUserIDStmt.QueryRowContext(pCtx, pUserID).Scan(&access.ID, &access.CreationTime, &access.LastUpdated, &access.Deleted, &access.Version, &access.UserID, &access.MostRecentBlock, &access.RequiredTokensOwned, &access.IsAdmin)
	if err != nil {
		if err == sql.ErrNoRows {
			return persist.Access{}, persist.ErrAccessNotFoundByUserID{UserID: pUserID}
		}
		return persist.Access{}, err
	}

	return access, nil
}

// HasRequiredTokens returns true if the user has the required tokens
func (a *AccessRepository) HasRequiredTokens(pCtx context.Context, pUserID persist.DBID, pTokenIdentifiers []persist.TokenIdentifiers) (bool, error) {
	var accessID persist.DBID
	err := a.hasRequiredTokensStmt.QueryRowContext(pCtx, pUserID, pq.Array(pTokenIdentifiers)).Scan(&accessID)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// UpsertRequiredTokensByUserID upserts the given access required tokens for the given user ID
func (a *AccessRepository) UpsertRequiredTokensByUserID(pCtx context.Context, pUserID persist.DBID, pRequiredtokens map[persist.TokenIdentifiers]uint64, pBlock persist.BlockNumber) error {
	_, err := a.upsertRequiredTokensByUserIDStmt.ExecContext(pCtx, persist.GenerateID(), pUserID, pBlock, pq.Array(pRequiredtokens), false)
	return err
}
