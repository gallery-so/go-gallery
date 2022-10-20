package postgres

import (
	"context"
	"database/sql"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

// MembershipRepository is a repository for storing membership information in the database
type MembershipRepository struct {
	db                  *sql.DB
	queries             *db.Queries
	upsertByTokenIDStmt *sql.Stmt
	getByTokenIDStmt    *sql.Stmt
	getAllStmt          *sql.Stmt
}

// NewMembershipRepository creates a new postgres repository for interacting with tiers
func NewMembershipRepository(db *sql.DB, queries *db.Queries) *MembershipRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	upsertByTokenIDStmt, err := db.PrepareContext(ctx, `INSERT INTO membership (ID,TOKEN_ID,NAME,ASSET_URL,OWNERS,LAST_UPDATED) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (TOKEN_ID) DO UPDATE SET NAME = EXCLUDED.NAME, ASSET_URL = EXCLUDED.ASSET_URL, OWNERS = EXCLUDED.OWNERS, LAST_UPDATED = EXCLUDED.LAST_UPDATED;`)
	checkNoErr(err)

	getByTokenIDStmt, err := db.PrepareContext(ctx, `SELECT ID,CREATED_AT,LAST_UPDATED,VERSION,NAME,ASSET_URL,OWNERS FROM membership WHERE TOKEN_ID = $1 AND DELETED = false;`)
	checkNoErr(err)

	getAllStmt, err := db.PrepareContext(ctx, `SELECT ID,TOKEN_ID,NAME,ASSET_URL,OWNERS,CREATED_AT,LAST_UPDATED FROM membership WHERE DELETED = false;`)
	checkNoErr(err)

	return &MembershipRepository{db: db, queries: queries, upsertByTokenIDStmt: upsertByTokenIDStmt, getByTokenIDStmt: getByTokenIDStmt, getAllStmt: getAllStmt}
}

// UpsertByTokenID upserts the given tier
func (m *MembershipRepository) UpsertByTokenID(pCtx context.Context, pTokenID persist.TokenID, pTier persist.MembershipTier) error {
	_, err := m.upsertByTokenIDStmt.ExecContext(pCtx, persist.GenerateID(), pTier.TokenID, pTier.Name, pTier.AssetURL, pq.Array(pTier.Owners), pTier.LastUpdated)
	return err
}

// GetByTokenID returns the tier with the given token ID
func (m *MembershipRepository) GetByTokenID(pCtx context.Context, pTokenID persist.TokenID) (persist.MembershipTier, error) {
	tier := persist.MembershipTier{TokenID: pTokenID}
	err := m.getByTokenIDStmt.QueryRowContext(pCtx, pTokenID).Scan(&tier.ID, &tier.CreationTime, &tier.LastUpdated, &tier.Version, &tier.Name, &tier.AssetURL, pq.Array(&tier.Owners))
	if err != nil {
		if err == sql.ErrNoRows {
			return persist.MembershipTier{}, persist.ErrMembershipNotFoundByTokenID{TokenID: pTokenID}
		}
		return persist.MembershipTier{}, err
	}

	return tier, nil
}

// GetAll returns all the tiers
func (m *MembershipRepository) GetAll(pCtx context.Context) ([]persist.MembershipTier, error) {
	rows, err := m.getAllStmt.QueryContext(pCtx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tiers := make([]persist.MembershipTier, 0, 10)
	for rows.Next() {
		tier := persist.MembershipTier{}
		err := rows.Scan(&tier.ID, &tier.TokenID, &tier.Name, &tier.AssetURL, pq.Array(&tier.Owners), &tier.CreationTime, &tier.LastUpdated)
		if err != nil {
			return nil, err
		}
		tiers = append(tiers, tier)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return tiers, nil
}
