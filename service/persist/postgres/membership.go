package postgres

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

// MembershipRepository is a repository for storing membership information in the database
type MembershipRepository struct {
	db *sql.DB
}

// NewMembershipRepository creates a new postgres repository for interacting with tiers
func NewMembershipRepository(db *sql.DB) *MembershipRepository {
	return &MembershipRepository{db: db}
}

// UpsertByTokenID upserts the given tier
func (m *MembershipRepository) UpsertByTokenID(pCtx context.Context, pTokenID persist.TokenID, pTier persist.MembershipTier) error {
	sqlStr := `INSERT INTO membership (ID,TOKEN_ID,NAME,ASSET_URL,OWNERS) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (TOKEN_ID) DO UPDATE SET NAME = $3, ASSET_URL = $4, OWNERS = $5`
	_, err := m.db.ExecContext(pCtx, sqlStr, persist.GenerateID(), pTier.TokenID, pTier.Name, pTier.AssetURL, pq.Array(pTier.Owners))
	return err
}

// GetByTokenID returns the tier with the given token ID
func (m *MembershipRepository) GetByTokenID(pCtx context.Context, pTokenID persist.TokenID) (persist.MembershipTier, error) {
	sqlStr := `SELECT ID,CREATED_AT,LAST_UPDATED,DELETED,VERSION,NAME,ASSET_URL,OWNERS FROM membership WHERE TOKEN_ID = $1`
	tier := persist.MembershipTier{TokenID: pTokenID}
	err := m.db.QueryRowContext(pCtx, sqlStr, pTokenID).Scan(&tier.ID, &tier.CreationTime, &tier.LastUpdated, &tier.Deleted, &tier.Deleted, &tier.Name, &tier.AssetURL, &tier.Owners)
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
	sqlStr := `SELECT ID,TOKEN_ID,NAME,ASSET_URL,OWNERS FROM membership`
	rows, err := m.db.QueryContext(pCtx, sqlStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tiers := make([]persist.MembershipTier, 0, 10)
	for rows.Next() {
		tier := persist.MembershipTier{}
		err := rows.Scan(&tier.ID, &tier.TokenID, &tier.Name, &tier.AssetURL, &tier.Owners)
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
