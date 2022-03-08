package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

// CommunityRepository is a repository for storing community information in the database
type CommunityRepository struct {
	db                   *sql.DB
	upsertByContractStmt *sql.Stmt
	getByContractStmt    *sql.Stmt
	getAllStmt           *sql.Stmt
}

// NewCommunityRepository creates a new postgres repository for interacting with communities
func NewCommunityRepository(db *sql.DB) *CommunityRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	upsertByContractStmt, err := db.PrepareContext(ctx, `INSERT INTO community (ID,CONTRACT_ADDRESS,TOKEN_ID_RANGES,NAME,DESCRIPTION,PROFILE_IMAGE_URL,BANNER_IMAGE_URL,OWNERS,LAST_UPDATED) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (CONTRACT_ADDRESS) DO UPDATE SET NAME = EXCLUDED.NAME, DESCRIPTION = EXCLUDED.DESCRIPTION, BANNER_IMAGE_URL = EXCLUDED.BANNER_IMAGE_URL, PROFILE_IMAGE_URL = EXCLUDED.PROFILE_IMAGE_URL, OWNERS = EXCLUDED.OWNERS, LAST_UPDATED = EXCLUDED.LAST_UPDATED;`)
	checkNoErr(err)

	getByContractStmt, err := db.PrepareContext(ctx, `SELECT ID,CREATED_AT,LAST_UPDATED,VERSION,TOKEN_ID_RANGES,NAME,DESCRIPTION,BANNER_IMAGE_URL,PROFILE_IMAGE_URL,OWNERS FROM community WHERE CONTRACT_ADDRESS = $1 AND DELETED = false;`)
	checkNoErr(err)

	getAllStmt, err := db.PrepareContext(ctx, `SELECT ID,CONTRACT_ADDRESS,TOKEN_ID_RANGES,NAME,DESCRIPTION,BANNER_IMAGE_URL,PROFILE_IMAGE_URL,OWNERS,CREATED_AT,LAST_UPDATED FROM community WHERE DELETED = false;`)
	checkNoErr(err)

	return &CommunityRepository{db: db, upsertByContractStmt: upsertByContractStmt, getByContractStmt: getByContractStmt, getAllStmt: getAllStmt}
}

// UpsertByContract upserts the given tier
func (m *CommunityRepository) UpsertByContract(pCtx context.Context, pAddress persist.Address, pTier persist.Community) error {
	_, err := m.upsertByContractStmt.ExecContext(pCtx, persist.GenerateID(), pAddress, pTier.TokenIDRanges, pTier.Name, pTier.Description, pTier.BannerImageURL, pTier.ProfileImageURL, pq.Array(pTier.Owners), pTier.LastUpdated)
	return err
}

// GetByContract returns the tier with the given token ID
func (m *CommunityRepository) GetByContract(pCtx context.Context, pContractAddress persist.Address) (persist.Community, error) {
	tier := persist.Community{ContractAddress: pContractAddress}
	err := m.getByContractStmt.QueryRowContext(pCtx, pContractAddress).Scan(&tier.ID, &tier.CreationTime, &tier.LastUpdated, &tier.Version, pq.Array(&tier.TokenIDRanges), &tier.Name, &tier.Description, &tier.BannerImageURL, &tier.ProfileImageURL, pq.Array(&tier.Owners))
	if err != nil {
		if err == sql.ErrNoRows {
			return persist.Community{}, persist.ErrCommunityNotFoundByAddress{ContractAddress: pContractAddress}
		}
		return persist.Community{}, err
	}

	return tier, nil
}

// GetAll returns all the tiers
func (m *CommunityRepository) GetAll(pCtx context.Context) ([]persist.Community, error) {
	rows, err := m.getAllStmt.QueryContext(pCtx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tiers := make([]persist.Community, 0, 10)
	for rows.Next() {
		tier := persist.Community{}
		err := rows.Scan(&tier.ID, &tier.ContractAddress, pq.Array(&tier.TokenIDRanges), &tier.Name, &tier.Description, &tier.BannerImageURL, &tier.ProfileImageURL, pq.Array(&tier.Owners), &tier.CreationTime, &tier.LastUpdated)
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
