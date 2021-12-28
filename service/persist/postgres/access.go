package postgres

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

// AccessRepository represents an access repository in the postgres database
type AccessRepository struct {
	db *sql.DB
}

// NewAccessRepository creates a new postgres repository for interacting with accesses
func NewAccessRepository(db *sql.DB) *AccessRepository {
	return &AccessRepository{db: db}
}

// GetByUserID returns the access for the given user ID
func (a *AccessRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID) (persist.Access, error) {
	sqlStr := `SELECT ID,USER_ID,MOST_RECENT_BLOCK,REQUIRED_TOKENS_OWNED,IS_ADMIN FROM access WHERE USER_ID = $1 LIMIT 1`
	access := persist.Access{}
	err := a.db.QueryRowContext(pCtx, sqlStr, pUserID).Scan(&access.ID, &access.UserID, &access.MostRecentBlock, &access.RequiredTokensOwned, &access.IsAdmin)
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
	sqlStr := `SELECT ID FROM access WHERE USER_ID = $1 AND MOST_RECENT_BLOCK = (SELECT MAX(MOST_RECENT_BLOCK) FROM access WHERE USER_ID = $1) AND REQUIRED_TOKENS -> ALL($2) IS NOT NULL`
	var accessID persist.DBID
	err := a.db.QueryRowContext(pCtx, sqlStr, pUserID, pq.Array(pTokenIdentifiers)).Scan(&accessID)
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
	sqlStr := `INSERT INTO access (ID,USER_ID,MOST_RECENT_BLOCK,REQUIRED_TOKENS,IS_ADMIN) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (USER_ID) DO UPDATE SET REQUIRED_TOKENS = $4, MOST_RECENT_BLOCK = $3`
	_, err := a.db.ExecContext(pCtx, sqlStr, persist.GenerateID(), pUserID, pBlock, pq.Array(pRequiredtokens), false)
	return err
}
